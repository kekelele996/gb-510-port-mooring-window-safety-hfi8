package dto

import (
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
)

// PlanImpact describes a 系泊方案 sharing the same facility and related code as
// a weather window, annotated with how the current window state affects it.
type PlanImpact struct {
	model.MooringPlan
	Impact string `json:"impact"`
}

// ClearanceImpact describes a 安全许可 bound to the weather window context,
// annotated with the action the field team must take on the current version.
type ClearanceImpact struct {
	model.SafetyClearance
	Impact string `json:"impact"`
}

// WindowImpactAssessment is the payload of GET /weather-windows/:id/impact. It
// lists every plan and clearance in the same 作业区/关联事项 scope and gives the
// duty officer a single continue-or-suspend conclusion.
type WindowImpactAssessment struct {
	Window            model.WeatherWindow `json:"window"`
	Plans             []PlanImpact        `json:"plans"`
	Clearances        []ClearanceImpact   `json:"clearances"`
	ExpiredClearances int                 `json:"expiredClearances"`
	ReboundClearances int                 `json:"reboundClearances"`
	BlockedClearances int                 `json:"blockedClearances"`
	ReadyClearances   int                 `json:"readyClearances"`
	Decision          string              `json:"decision"`
	DecisionReason    string              `json:"decisionReason"`
	AssessedAt        time.Time           `json:"assessedAt"`
}

const (
	WindowDecisionContinue = "continue"
	WindowDecisionSuspend  = "suspend"
)
