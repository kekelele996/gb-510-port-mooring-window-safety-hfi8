package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/config"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newClearanceTestService(t *testing.T) (*gorm.DB, SafetyClearanceService, SecurityService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.SafetyClearance{}, &model.WeatherWindow{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	clearanceRepository := repository.NewSafetyClearanceRepository(db)
	windowRepository := repository.NewWeatherWindowRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewSafetyClearanceService(clearanceRepository, windowRepository, security)
	return db, svc, security
}

func seedSafeWindow(t *testing.T, db *gorm.DB, code string, version uint, status string) {
	t.Helper()
	window := model.WeatherWindow{
		BaseModel: model.BaseModel{Code: code, Name: "Test window", Status: status, Version: version},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "low",
		EffectiveAt: time.Now().UTC(), Evidence: "checked", RelatedCode: "REL-TEST",
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create window: %v", err)
	}
}

func TestSafetyClearanceRequiresIndependentReviewer(t *testing.T) {
	db, svc, security := newClearanceTestService(t)
	seedSafeWindow(t, db, "WW-TEST", 7, constants.WeatherWindowSafe)

	clearanceRepository := repository.NewSafetyClearanceRepository(db)
	item := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-TEST", Name: "Test clearance", Status: model.SafetyClearanceInitialStatus, Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: time.Now().UTC(), Evidence: "checked", RelatedCode: "REL-TEST",
		WindowCode: "WW-TEST", WindowVersion: 1,
	}
	if err := clearanceRepository.Create(context.Background(), &item); err != nil {
		t.Fatalf("create clearance: %v", err)
	}

	first, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "operator safety submission", WindowVersion: 7,
	}, "operator", model.RoleOperator, "request-submit")
	if err != nil {
		t.Fatalf("first confirmation: %v", err)
	}
	if first.Status != "pending" || first.SubmittedBy != "operator" || first.ConfirmedBy != "" || first.WindowVersion != 7 {
		t.Fatalf("unexpected first confirmation state: %+v", first)
	}

	_, err = svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: first.Version, Reason: "attempted self approval", WindowVersion: 7,
	}, "operator", model.RoleReviewer, "request-self")
	if !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("expected self approval error, got %v", err)
	}

	_, err = svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: first.Version, Reason: "second operator approval", WindowVersion: 7,
	}, "operator-two", model.RoleOperator, "request-operator")
	if !errors.Is(err, ErrReviewerRequired) {
		t.Fatalf("expected reviewer role error, got %v", err)
	}

	final, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: first.Version, Reason: "independent safety review", WindowVersion: 7,
	}, "reviewer", model.RoleReviewer, "request-review")
	if err != nil {
		t.Fatalf("independent confirmation: %v", err)
	}
	if final.Status != "cleared" || final.SubmittedBy != "operator" || final.ConfirmedBy != "reviewer" {
		t.Fatalf("unexpected final confirmation state: %+v", final)
	}

	logs, total, err := security.ListAudits(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("expected two audit entries, total=%d len=%d", total, len(logs))
	}
	for _, audit := range logs {
		if audit.WindowVersion != 7 || audit.RequestID == "" || audit.Actor == "" {
			t.Fatalf("audit did not preserve confirmation context: %+v", audit)
		}
	}
}

func TestSafetyClearanceBlocksUnsafeOrStaleWindow(t *testing.T) {
	db, svc, _ := newClearanceTestService(t)
	seedSafeWindow(t, db, "WW-RESTRICTED", 1, constants.WeatherWindowRestricted)

	repo := repository.NewSafetyClearanceRepository(db)
	blocked := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-UNSAFE", Name: "Unsafe window clearance", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: time.Now().UTC(), WindowCode: "WW-RESTRICTED", WindowVersion: 1,
	}
	if err := repo.Create(context.Background(), &blocked); err != nil {
		t.Fatalf("create clearance: %v", err)
	}
	if _, err := svc.Transition(context.Background(), blocked.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "submit on unsafe window", WindowVersion: 1,
	}, "operator", model.RoleOperator, "request-unsafe"); !errors.Is(err, ErrWindowUnsafe) {
		t.Fatalf("expected unsafe window error, got %v", err)
	}

	seedSafeWindow(t, db, "WW-SAFE", 2, constants.WeatherWindowSafe)
	stale := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-STALE", Name: "Stale version clearance", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: time.Now().UTC(), WindowCode: "WW-SAFE", WindowVersion: 1,
	}
	if err := repo.Create(context.Background(), &stale); err != nil {
		t.Fatalf("create clearance: %v", err)
	}
	if _, err := svc.Transition(context.Background(), stale.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "submit against old version", WindowVersion: 1,
	}, "operator", model.RoleOperator, "request-stale"); !errors.Is(err, ErrVersionChanged) {
		t.Fatalf("expected version changed error, got %v", err)
	}
}

func TestReboundClearanceRequiresResubmitThenIndependentReview(t *testing.T) {
	db, svc, _ := newClearanceTestService(t)
	// A submitted pending clearance was anchored to v1 when the window moved.
	seedSafeWindow(t, db, "WW-MOVE", 2, constants.WeatherWindowSafe)
	repo := repository.NewSafetyClearanceRepository(db)
	now := time.Now().UTC()
	item := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-REBIND", Name: "Rebound clearance", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, WindowCode: "WW-MOVE", WindowVersion: 2,
		RebindRequired: true, SubmittedBy: "operator", SubmittedAt: &now,
	}
	if err := repo.Create(context.Background(), &item); err != nil {
		t.Fatalf("create clearance: %v", err)
	}

	// A reviewer cannot directly release the old rebound submission.
	if _, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "reviewer tries to release old submission", WindowVersion: 2,
	}, "reviewer", model.RoleReviewer, "request-block"); !errors.Is(err, ErrRebindRequired) {
		t.Fatalf("expected rebind required error, got %v", err)
	}

	// Re-submitting on a stale version is still rejected.
	if _, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "resubmit with stale version", WindowVersion: 1,
	}, "operator", model.RoleOperator, "request-stale-resubmit"); !errors.Is(err, ErrVersionChanged) {
		t.Fatalf("expected version changed error on resubmit, got %v", err)
	}

	resubmitted, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "resubmit against latest window", WindowVersion: 2,
	}, "operator", model.RoleOperator, "request-resubmit")
	if err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if resubmitted.RebindRequired || resubmitted.SubmittedBy != "operator" || resubmitted.WindowVersion != 2 || resubmitted.ConfirmedBy != "" {
		t.Fatalf("unexpected resubmitted state: %+v", resubmitted)
	}

	cleared, err := svc.Transition(context.Background(), item.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: resubmitted.Version, Reason: "independent review after resubmit", WindowVersion: 2,
	}, "reviewer", model.RoleReviewer, "request-final-review")
	if err != nil {
		t.Fatalf("final review: %v", err)
	}
	if cleared.Status != "cleared" || cleared.ConfirmedBy != "reviewer" {
		t.Fatalf("unexpected cleared state: %+v", cleared)
	}
}
