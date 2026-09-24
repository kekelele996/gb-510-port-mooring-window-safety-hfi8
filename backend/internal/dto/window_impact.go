package dto

import "github.com/blueship581/port-mooring-window-safety/backend/internal/model"

// ImpactedPlan is a 系泊方案 projected into a window impact assessment. Records
// are matched by 同作业区 (facility) + 同一关联事项 (relatedCode).
type ImpactedPlan struct {
	ID             uint   `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	RiskLevel      string `json:"riskLevel"`
	Owner          string `json:"owner"`
	Version        uint   `json:"version"`
	Recommendation string `json:"recommendation"`
}

// ImpactedClearance is a 安全许可 projected into a window impact assessment. A
// clearance belongs to a window either via relatedCode = window.Code or via the
// shared 关联事项 code, and must share the same 作业区.
type ImpactedClearance struct {
	ID             uint   `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	RiskLevel      string `json:"riskLevel"`
	Owner          string `json:"owner"`
	Version        uint   `json:"version"`
	WindowVersion  uint   `json:"windowVersion"`
	SubmittedBy    string `json:"submittedBy"`
	ConfirmedBy    string `json:"confirmedBy"`
	StaleVersion   bool   `json:"staleVersion"`
	Recommendation string `json:"recommendation"`
}

// WindowImpact is the impact assessment attached to a window detail. The
// decision is the explicit on-site 继续/暂停 conclusion driven by window state.
type WindowImpact struct {
	Window         model.WeatherWindow `json:"window"`
	Decision       string              `json:"decision"`
	DecisionReason string              `json:"decisionReason"`
	PlanCount      int                 `json:"planCount"`
	ClearanceCount int                 `json:"clearanceCount"`
	ExpiredCount   int                 `json:"expiredCount"`
	StaleCount     int                 `json:"staleCount"`
	Plans          []ImpactedPlan      `json:"plans"`
	Clearances     []ImpactedClearance `json:"clearances"`
}
