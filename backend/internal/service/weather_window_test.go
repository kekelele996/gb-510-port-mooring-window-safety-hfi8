package service

import (
	"context"
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

func newWindowTestService(t *testing.T) (*gorm.DB, WeatherWindowService, SafetyClearanceService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.WeatherWindow{}, &model.MooringPlan{}, &model.SafetyClearance{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	windowRepo := repository.NewWeatherWindowRepository(db)
	planRepo := repository.NewMooringPlanRepository(db)
	clearanceRepo := repository.NewSafetyClearanceRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	windowService := NewWeatherWindowService(windowRepo, planRepo, clearanceRepo, security)
	clearanceService := NewSafetyClearanceService(clearanceRepo, windowRepo, security)
	return db, windowService, clearanceService
}

func TestWindowRestrictedCascadesClearancesAndImpact(t *testing.T) {
	db, windowService, _ := newWindowTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	window := model.WeatherWindow{
		BaseModel: model.BaseModel{Code: "WW-C1", Name: "Cascade window", Status: "safe", Version: 1},
		Facility:  "Berth A", Owner: "ops", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-C1",
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create window: %v", err)
	}
	plan := model.MooringPlan{
		BaseModel: model.BaseModel{Code: "MP-C1", Name: "Cascade plan", Status: "approved", Version: 1},
		Facility:  "Berth A", Owner: "ops", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-C1",
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	cleared := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-C1", Name: "Released", Status: "cleared", Version: 1},
		Facility:  "Berth A", Owner: "ops", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-C1", WindowCode: "WW-C1", WindowVersion: 1,
		SubmittedBy: "operator", ConfirmedBy: "reviewer",
	}
	pendingSubmit := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-C2", Name: "Under review", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "ops", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-C1", WindowCode: "WW-C1", WindowVersion: 1,
		SubmittedBy: "operator", SubmittedAt: &now,
	}
	pendingFresh := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-C3", Name: "Not submitted", Status: "pending", Version: 1},
		Facility:  "Berth A", Owner: "ops", Category: "test", RiskLevel: "medium",
		EffectiveAt: now, RelatedCode: "REL-C1", WindowCode: "WW-C1", WindowVersion: 1,
	}
	for _, item := range []*model.SafetyClearance{&cleared, &pendingSubmit, &pendingFresh} {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("create clearance: %v", err)
		}
	}

	updated, err := windowService.Transition(ctx, window.ID, dto.TransitionRequest{
		Status: constants.WeatherWindowRestricted, ExpectedVersion: 1, Reason: "wind picked up",
	}, "reviewer", "request-cascade")
	if err != nil {
		t.Fatalf("transition window: %v", err)
	}
	if updated.Status != "restricted" || updated.Version != 2 {
		t.Fatalf("unexpected window state: %+v", updated)
	}

	var expired model.SafetyClearance
	if err := db.First(&expired, cleared.ID).Error; err != nil {
		t.Fatalf("reload expired clearance: %v", err)
	}
	if expired.Status != "expired" {
		t.Fatalf("released clearance must expire, got %s", expired.Status)
	}

	var rebound model.SafetyClearance
	if err := db.First(&rebound, pendingSubmit.ID).Error; err != nil {
		t.Fatalf("reload rebound clearance: %v", err)
	}
	if !rebound.RebindRequired || rebound.WindowVersion != 2 || rebound.SubmittedBy != "operator" || rebound.Status != "pending" {
		t.Fatalf("submitted pending clearance must rebind and keep submitter: %+v", rebound)
	}

	var fresh model.SafetyClearance
	if err := db.First(&fresh, pendingFresh.ID).Error; err != nil {
		t.Fatalf("reload fresh clearance: %v", err)
	}
	if fresh.RebindRequired || fresh.WindowVersion != 1 {
		t.Fatalf("unsubmitted pending clearance must not be rebound: %+v", fresh)
	}

	assessment, err := windowService.ImpactAssessment(ctx, window.ID)
	if err != nil {
		t.Fatalf("impact assessment: %v", err)
	}
	if assessment.Decision != dto.WindowDecisionSuspend {
		t.Fatalf("restricted window must suspend field work, got %s (%s)", assessment.Decision, assessment.DecisionReason)
	}
	if assessment.ExpiredClearances != 1 || assessment.ReboundClearances != 1 || len(assessment.Plans) != 1 {
		t.Fatalf("unexpected assessment counts: %+v", assessment)
	}
}

func TestSafeWindowImpactContinuesWhenReady(t *testing.T) {
	db, windowService, _ := newWindowTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	window := model.WeatherWindow{
		BaseModel: model.BaseModel{Code: "WW-G1", Name: "Good window", Status: "safe", Version: 3},
		Facility:  "Berth B", Owner: "ops", Category: "test", RiskLevel: "low",
		EffectiveAt: now, RelatedCode: "REL-G1",
	}
	plan := model.MooringPlan{
		BaseModel: model.BaseModel{Code: "MP-G1", Name: "Ready plan", Status: "approved", Version: 1},
		Facility:  "Berth B", Owner: "ops", Category: "test", RiskLevel: "low",
		EffectiveAt: now, RelatedCode: "REL-G1",
	}
	ready := model.SafetyClearance{
		BaseModel: model.BaseModel{Code: "SC-G1", Name: "Released current", Status: "cleared", Version: 1},
		Facility:  "Berth B", Owner: "ops", Category: "test", RiskLevel: "low",
		EffectiveAt: now, RelatedCode: "REL-G1", WindowCode: "WW-G1", WindowVersion: 3,
		SubmittedBy: "operator", ConfirmedBy: "reviewer",
	}
	for _, item := range []any{&window, &plan, &ready} {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	assessment, err := windowService.ImpactAssessment(ctx, window.ID)
	if err != nil {
		t.Fatalf("impact assessment: %v", err)
	}
	if assessment.Decision != dto.WindowDecisionContinue || assessment.ReadyClearances != 1 {
		t.Fatalf("expected continue with one ready clearance, got %s ready=%d reason=%s", assessment.Decision, assessment.ReadyClearances, assessment.DecisionReason)
	}
}
