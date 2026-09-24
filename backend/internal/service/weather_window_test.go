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

const impactFacility = "Berth Impact"

func newWindowTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.WeatherWindow{}, &model.MooringPlan{}, &model.SafetyClearance{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func newWindowService(db *gorm.DB) (WeatherWindowService, SafetyClearanceService) {
	windowRepo := repository.NewWeatherWindowRepository(db)
	planRepo := repository.NewMooringPlanRepository(db)
	clearanceRepo := repository.NewSafetyClearanceRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	windowSvc := NewWeatherWindowService(windowRepo, planRepo, clearanceRepo, security)
	clearanceSvc := NewSafetyClearanceService(clearanceRepo, windowRepo, security)
	return windowSvc, clearanceSvc
}

func seedImpactFixture(t *testing.T, db *gorm.DB) (model.WeatherWindow, model.SafetyClearance, model.SafetyClearance, model.MooringPlan) {
	t.Helper()
	now := time.Now().UTC()
	window := model.WeatherWindow{
		BaseModel: model.BaseModel{Code: "WW-IMP", Name: "Impact window", Status: "safe", Version: 1},
		Facility:  impactFacility, Owner: "ops", Category: "berthing", RiskLevel: "low",
		EffectiveAt: now, Evidence: "calm", RelatedCode: "REL-IMP",
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create window: %v", err)
	}
	cleared := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-CLR", Name: "Cleared", Status: "cleared", Version: 1},
		Facility:  impactFacility, Owner: "ops", Category: "berthing", RiskLevel: "low",
		EffectiveAt: now, RelatedCode: "WW-IMP", WindowVersion: 1,
		SubmittedBy: "operator", SubmittedAt: &now, ConfirmedBy: "reviewer", ConfirmedAt: &now,
	}
	pending := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-PND", Name: "Pending review", Status: "pending", Version: 1},
		Facility:  impactFacility, Owner: "ops", Category: "berthing", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-IMP", WindowVersion: 1,
		SubmittedBy: "operator", SubmittedAt: &now,
	}
	other := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-OTHER", Name: "Other area", Status: "cleared", Version: 1},
		Facility:  "Other Berth", Owner: "ops", Category: "berthing", RiskLevel: "low",
		EffectiveAt: now, RelatedCode: "WW-IMP", WindowVersion: 1,
	}
	plan := model.MooringPlan{
		BaseModel: model.BaseModel{Code: "MP-IMP", Name: "Mooring plan", Status: "approved", Version: 1},
		Facility:  impactFacility, Owner: "ops", Category: "berthing", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-IMP",
	}
	for _, item := range []interface{}{&cleared, &pending, &other, &plan} {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
	return window, cleared, pending, plan
}

func TestWindowRestrictionCascadesClearancesAndImpact(t *testing.T) {
	db := newWindowTestDB(t)
	windowSvc, _ := newWindowService(db)
	window, cleared, pending, plan := seedImpactFixture(t, db)

	// Safe window impact: continue.
	impact, err := windowSvc.Impact(context.Background(), window.ID)
	if err != nil {
		t.Fatalf("impact safe: %v", err)
	}
	if impact.Decision != "continue" || impact.PlanCount != 1 || impact.ClearanceCount != 2 {
		t.Fatalf("unexpected safe impact: %+v", impact)
	}
	if len(impact.Plans) != 1 || impact.Plans[0].Code != plan.Code {
		t.Fatalf("impact should list the same-scope plan: %+v", impact.Plans)
	}

	// Restrict the window.
	restricted, err := windowSvc.Transition(context.Background(), window.ID, dto.TransitionRequest{
		Status: "restricted", ExpectedVersion: 1, Reason: "wind exceeds threshold",
	}, "operator", "req-restrict")
	if err != nil {
		t.Fatalf("restrict window: %v", err)
	}
	if restricted.Version != 2 {
		t.Fatalf("expected window v2 after transition, got %d", restricted.Version)
	}

	var expired model.SafetyClearance
	if err := db.First(&expired, cleared.ID).Error; err != nil {
		t.Fatalf("reload cleared clearance: %v", err)
	}
	if expired.Status != "expired" {
		t.Fatalf("released clearance must expire with window, got %s", expired.Status)
	}
	if expired.ConfirmedBy != "reviewer" {
		t.Fatalf("historical confirmer should be retained, got %q", expired.ConfirmedBy)
	}

	var rebound model.SafetyClearance
	if err := db.First(&rebound, pending.ID).Error; err != nil {
		t.Fatalf("reload pending clearance: %v", err)
	}
	if rebound.Status != "pending" || rebound.SubmittedBy != "operator" || rebound.WindowVersion != 2 {
		t.Fatalf("pending clearance should keep submitter and rebind v2: %+v", rebound)
	}
	if rebound.Version != 2 {
		t.Fatalf("rebind must bump optimistic version, got %d", rebound.Version)
	}

	// Unrelated-area clearance must be untouched.
	var untouched model.SafetyClearance
	if err := db.Where("code = ?", "SC-OTHER").First(&untouched).Error; err != nil {
		t.Fatalf("load other: %v", err)
	}
	if untouched.Status != "cleared" {
		t.Fatalf("clearance in another facility must not cascade: %s", untouched.Status)
	}

	// Restricted window impact: suspend, one expired, one stale-bound clearance.
	impact, err = windowSvc.Impact(context.Background(), window.ID)
	if err != nil {
		t.Fatalf("impact restricted: %v", err)
	}
	if impact.Decision != "suspend" || impact.ExpiredCount != 1 || impact.StaleCount != 1 {
		t.Fatalf("unexpected restricted impact: %+v", impact)
	}

	// Cascade audits record the clearance's own before-state, not the window's.
	var rebind model.AuditLog
	if err := db.Where("action = ?", "window_cascade_rebind").First(&rebind).Error; err != nil {
		t.Fatalf("load rebind audit: %v", err)
	}
	if rebind.BeforeState != "pending" || rebind.AfterState != "pending" || rebind.WindowVersion != 2 {
		t.Fatalf("rebind audit must record clearance pending state and window v2: %+v", rebind)
	}
	var expireAudit model.AuditLog
	if err := db.Where("action = ?", "window_cascade_expire").First(&expireAudit).Error; err != nil {
		t.Fatalf("load expire audit: %v", err)
	}
	if expireAudit.BeforeState != "cleared" || expireAudit.AfterState != "expired" {
		t.Fatalf("expire audit must record cleared -> expired: %+v", expireAudit)
	}
	for _, item := range impact.Clearances {
		switch item.Code {
		case "SC-CLR":
			// Expired release pinned to v1 is stale against current v2.
			if !item.StaleVersion || item.Status != "expired" {
				t.Fatalf("expired clearance should be flagged stale: %+v", item)
			}
		case "SC-PND":
			// Rebound pending clearance is pinned to the new version, but the
			// recommendation still forces an independent re-confirmation.
			if item.StaleVersion || item.SubmittedBy != "operator" {
				t.Fatalf("rebound pending clearance should keep submitter and match version: %+v", item)
			}
		}
	}
}

func TestOldSubmissionCannotReleaseReboundClearance(t *testing.T) {
	db := newWindowTestDB(t)
	windowSvc, clearanceSvc := newWindowService(db)
	window, _, pending, _ := seedImpactFixture(t, db)

	if _, err := windowSvc.Transition(context.Background(), window.ID, dto.TransitionRequest{
		Status: "restricted", ExpectedVersion: 1, Reason: "wind",
	}, "operator", "req-r"); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	// Recovery back to safe bumps window to v3; clearance is bound to v2.
	if _, err := windowSvc.Transition(context.Background(), window.ID, dto.TransitionRequest{
		Status: "safe", ExpectedVersion: 2, Reason: "wind recovered",
	}, "operator", "req-s"); err != nil {
		t.Fatalf("recover: %v", err)
	}

	// Reviewer's request still carrying the stale v2 window version: blocked.
	if _, err := clearanceSvc.Transition(context.Background(), pending.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 2, Reason: "stale reviewer release", WindowVersion: 2,
	}, "reviewer", model.RoleReviewer, "req-stale"); !errors.Is(err, ErrWindowVersion) {
		t.Fatalf("stale window version release must fail, got %v", err)
	}

	// Reviewer re-confirms explicitly against current v3: release succeeds and
	// the original submitter is preserved.
	final, err := clearanceSvc.Transition(context.Background(), pending.ID, dto.TransitionRequest{
		Status: "cleared", ExpectedVersion: 2, Reason: "rebind and release", WindowVersion: 3,
	}, "reviewer", model.RoleReviewer, "req-fresh")
	if err != nil {
		t.Fatalf("re-confirmed release: %v", err)
	}
	if final.Status != "cleared" || final.WindowVersion != 3 || final.SubmittedBy != "operator" || final.ConfirmedBy != "reviewer" {
		t.Fatalf("unexpected re-confirmed clearance: %+v", final)
	}
}
