package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/config"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newClearanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.SafetyClearance{}, &model.WeatherWindow{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func seedSafeWindow(t *testing.T, db *gorm.DB, code string, version uint) model.WeatherWindow {
	t.Helper()
	window := model.WeatherWindow{
		BaseModel: model.BaseModel{
			Code: code, Name: "Safe window " + code, Status: "safe", Version: version,
		},
		Facility: "Berth A", Owner: "operations", Category: "test", RiskLevel: "low",
		EffectiveAt: time.Now().UTC(), Evidence: "calm sea", RelatedCode: "REL-" + code,
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create window: %v", err)
	}
	return window
}

func TestSafetyClearanceRequiresIndependentReviewer(t *testing.T) {
	db := newClearanceTestDB(t)
	seedSafeWindow(t, db, "WW-TEST", 7)

	clearanceRepository := repository.NewSafetyClearanceRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewSafetyClearanceService(clearanceRepository, repository.NewWeatherWindowRepository(db), security)
	item := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-TEST", Name: "Test clearance", Status: model.SafetyClearanceInitialStatus, Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: time.Now().UTC(), Evidence: "checked", RelatedCode: "WW-TEST", WindowVersion: 1,
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

func TestSafetyClearanceRejectsUnsafeOrMissingWindow(t *testing.T) {
	db := newClearanceTestDB(t)
	// Restricted window: clearance release must be blocked.
	restricted := seedSafeWindow(t, db, "WW-UNSAFE", 1)
	restricted.Status = "restricted"
	if err := db.Save(&restricted).Error; err != nil {
		t.Fatalf("restrict window: %v", err)
	}

	clearanceRepository := repository.NewSafetyClearanceRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewSafetyClearanceService(clearanceRepository, repository.NewWeatherWindowRepository(db), security)

	blocked := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-UNSAFE", Name: "Unsafe", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "high",
		EffectiveAt: time.Now().UTC(), RelatedCode: "WW-UNSAFE", WindowVersion: 1,
	}
	if err := clearanceRepository.Create(context.Background(), &blocked); err != nil {
		t.Fatalf("create blocked clearance: %v", err)
	}
	if _, err := svc.Transition(context.Background(), blocked.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "attempt on restricted window", WindowVersion: 1,
	}, "operator", model.RoleOperator, "req-unsafe"); !errors.Is(err, ErrWindowUnsafe) {
		t.Fatalf("expected unsafe window error, got %v", err)
	}

	// Clearance without any linked window.
	orphan := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-ORPHAN", Name: "Orphan", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "low",
		EffectiveAt: time.Now().UTC(), RelatedCode: "WW-404", WindowVersion: 1,
	}
	if err := clearanceRepository.Create(context.Background(), &orphan); err != nil {
		t.Fatalf("create orphan clearance: %v", err)
	}
	if _, err := svc.Transition(context.Background(), orphan.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "attempt on missing window", WindowVersion: 1,
	}, "operator", model.RoleOperator, "req-missing"); !errors.Is(err, ErrWindowMissing) {
		t.Fatalf("expected missing window error, got %v", err)
	}
}

func TestRestrictedRecoveryReleaseRequiresSafeWindow(t *testing.T) {
	db := newClearanceTestDB(t)
	window := seedSafeWindow(t, db, "WW-REC", 1)
	window.Status = "restricted"
	if err := db.Save(&window).Error; err != nil {
		t.Fatalf("restrict window: %v", err)
	}

	repo := repository.NewSafetyClearanceRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewSafetyClearanceService(repo, repository.NewWeatherWindowRepository(db), security)

	restricted := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-REC", Name: "Restricted", Status: "restricted", Version: 1},
		Facility:  "Berth A", Owner: "operations", Category: "test", RiskLevel: "high",
		EffectiveAt: time.Now().UTC(), RelatedCode: "WW-REC", WindowVersion: 1,
	}
	if err := repo.Create(context.Background(), &restricted); err != nil {
		t.Fatalf("create restricted clearance: %v", err)
	}

	// Reviewer cannot release a restricted clearance while the window is unsafe.
	if _, err := svc.Transition(context.Background(), restricted.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "reviewer release on restricted window", WindowVersion: 1,
	}, "reviewer", model.RoleReviewer, "req-blocked"); !errors.Is(err, ErrWindowUnsafe) {
		t.Fatalf("restricted -> cleared must be blocked by unsafe window, got %v", err)
	}

	// Recover the window (v2), then release without windowVersion must fail, and
	// release pinned to the stale v1 must also fail.
	window.Status = "safe"
	window.Version = 2
	if err := db.Save(&window).Error; err != nil {
		t.Fatalf("recover window: %v", err)
	}
	if _, err := svc.Transition(context.Background(), restricted.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "release without window version",
	}, "reviewer", model.RoleReviewer, "req-noversion"); !errors.Is(err, ErrWindowVersion) {
		t.Fatalf("release without window version must fail, got %v", err)
	}
	if _, err := svc.Transition(context.Background(), restricted.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "stale version release", WindowVersion: 1,
	}, "reviewer", model.RoleReviewer, "req-stale"); !errors.Is(err, ErrWindowVersion) {
		t.Fatalf("stale window version release must fail, got %v", err)
	}
	released, err := svc.Transition(context.Background(), restricted.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 1, Reason: "recovered safe window release", WindowVersion: 2,
	}, "reviewer", model.RoleReviewer, "req-released")
	if err != nil {
		t.Fatalf("recovered release: %v", err)
	}
	if released.Status != "cleared" || released.WindowVersion != 2 {
		t.Fatalf("unexpected released clearance: %+v", released)
	}
}
