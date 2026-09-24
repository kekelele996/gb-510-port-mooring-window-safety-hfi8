package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
)

type WeatherWindowService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.WeatherWindow], error)
	Get(context.Context, uint) (model.WeatherWindow, error)
	Create(context.Context, dto.CreateWeatherWindow, string, string) (model.WeatherWindow, error)
	Update(context.Context, uint, dto.UpdateWeatherWindow, string, string) (model.WeatherWindow, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.WeatherWindow, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	ImpactAssessment(context.Context, uint) (dto.WindowImpactAssessment, error)
}

type weatherWindowService struct {
	repository repository.WeatherWindowRepository
	plans      repository.MooringPlanRepository
	clearances repository.SafetyClearanceRepository
	security   SecurityService
}

func NewWeatherWindowService(repo repository.WeatherWindowRepository, plans repository.MooringPlanRepository, clearances repository.SafetyClearanceRepository, security SecurityService) WeatherWindowService {
	return &weatherWindowService{repository: repo, plans: plans, clearances: clearances, security: security}
}

func (s *weatherWindowService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.WeatherWindow], error) {
	return s.repository.List(ctx, query)
}

func (s *weatherWindowService) Get(ctx context.Context, id uint) (model.WeatherWindow, error) {
	return s.repository.Get(ctx, id)
}

func (s *weatherWindowService) Create(ctx context.Context, input dto.CreateWeatherWindow, actor, requestID string) (model.WeatherWindow, error) {
	if err := validateWeatherWindowBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.WeatherWindow{}, err
	}
	item := model.WeatherWindow{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.WeatherWindowInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("create 风浪窗口: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "WeatherWindow", item.ID, "", item.Status, "created 风浪窗口")
	return item, nil
}

func (s *weatherWindowService) Update(ctx context.Context, id uint, input dto.UpdateWeatherWindow, actor, requestID string) (model.WeatherWindow, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.WeatherWindow{}, err
	}
	if err := validateWeatherWindowBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.WeatherWindow{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("update 风浪窗口: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "WeatherWindow", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *weatherWindowService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.WeatherWindow, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.WeatherWindow{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.WeatherWindowTransitions, current.Status, target) {
		return model.WeatherWindow{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	newVersion := input.ExpectedVersion + 1
	current.Status = target
	current.Version = newVersion
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("transition 风浪窗口: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "WeatherWindow", id, before, target, input.Reason); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("persist transition audit: %w", err)
	}
	// 窗口转差时同步处置关联许可：已放行的同步过期；待复核的换绑新窗口版本
	// 并保留提交人，旧提交不能直接放行。
	if target == constants.WeatherWindowRestricted || target == constants.WeatherWindowExpired {
		cascade, cascadeErr := s.clearances.CascadeWindowDowngrade(ctx, current.Code, current.Facility, current.RelatedCode, newVersion)
		if cascadeErr != nil {
			return model.WeatherWindow{}, fmt.Errorf("cascade window downgrade: %w", cascadeErr)
		}
		for _, expired := range cascade.Expired {
			_ = s.security.AuditWithWindowVersion(ctx, actor, requestID, "clearance_window_expire", "SafetyClearance", expired.ID, "cleared", "expired",
				fmt.Sprintf("窗口 %s 转为 %s，已放行许可同步过期", current.Code, target), newVersion)
		}
		for _, rebound := range cascade.Rebound {
			_ = s.security.AuditWithWindowVersion(ctx, actor, requestID, "clearance_window_rebind", "SafetyClearance", rebound.ID, "pending", "pending",
				fmt.Sprintf("窗口 %s 转为 %s，待复核许可换绑 v%d 并保留提交人 %s", current.Code, target, newVersion, rebound.SubmittedBy), newVersion)
		}
	}
	return s.repository.Get(ctx, id)
}

func (s *weatherWindowService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "WeatherWindow", id, current.Status, "deleted", "soft deleted 风浪窗口")
}

func (s *weatherWindowService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func (s *weatherWindowService) ImpactAssessment(ctx context.Context, id uint) (dto.WindowImpactAssessment, error) {
	window, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.WindowImpactAssessment{}, err
	}
	plans, err := s.plans.ListByScope(ctx, window.Facility, window.RelatedCode)
	if err != nil {
		return dto.WindowImpactAssessment{}, fmt.Errorf("load impacted plans: %w", err)
	}
	clearances, err := s.clearances.ListByWindow(ctx, window)
	if err != nil {
		return dto.WindowImpactAssessment{}, fmt.Errorf("load impacted clearances: %w", err)
	}

	assessment := dto.WindowImpactAssessment{
		Window:     window,
		Plans:      make([]dto.PlanImpact, 0, len(plans)),
		Clearances: make([]dto.ClearanceImpact, 0, len(clearances)),
		AssessedAt: time.Now().UTC(),
	}
	for _, plan := range plans {
		assessment.Plans = append(assessment.Plans, dto.PlanImpact{MooringPlan: plan, Impact: planImpact(window.Status, plan.Status)})
	}
	for _, clearance := range clearances {
		impact := clearanceImpact(window, clearance)
		assessment.Clearances = append(assessment.Clearances, dto.ClearanceImpact{SafetyClearance: clearance, Impact: impact})
		switch impact {
		case "expired":
			assessment.ExpiredClearances++
		case "rebind":
			assessment.ReboundClearances++
			assessment.BlockedClearances++
		case "blocked":
			assessment.BlockedClearances++
		case "ready":
			assessment.ReadyClearances++
		}
	}
	assessment.Decision, assessment.DecisionReason = windowDecision(window, plans, assessment)
	return assessment, nil
}

// planImpact labels a 系泊方案 with the on-site action implied by the window.
func planImpact(windowStatus, planStatus string) string {
	switch windowStatus {
	case constants.WeatherWindowSafe:
		if planStatus == "approved" {
			return "ready"
		}
		return "review"
	case constants.WeatherWindowRestricted:
		return "hold"
	case constants.WeatherWindowExpired:
		return "suspend"
	default:
		if planStatus == "approved" {
			return "hold"
		}
		return "watch"
	}
}

// clearanceImpact labels a 安全许可 as expired, rebound, blocked or ready.
func clearanceImpact(window model.WeatherWindow, clearance model.SafetyClearance) string {
	switch clearance.Status {
	case "expired":
		return "expired"
	case "restricted":
		return "blocked"
	case "cleared":
		if window.Status == constants.WeatherWindowSafe && clearance.WindowVersion == window.Version {
			return "ready"
		}
		return "blocked"
	case "pending":
		if clearance.RebindRequired || (clearance.SubmittedBy != "" && clearance.WindowVersion != window.Version) {
			return "rebind"
		}
		if window.Status == constants.WeatherWindowSafe {
			return "ready"
		}
		return "blocked"
	default:
		return "blocked"
	}
}

func windowDecision(window model.WeatherWindow, plans []model.MooringPlan, assessment dto.WindowImpactAssessment) (string, string) {
	switch window.Status {
	case constants.WeatherWindowRestricted:
		return dto.WindowDecisionSuspend, fmt.Sprintf("窗口为受限状态，%d 份许可已过期、%d 份待复核许可需重新确认，现场暂停靠泊系泊作业", assessment.ExpiredClearances, assessment.ReboundClearances)
	case constants.WeatherWindowExpired:
		return dto.WindowDecisionSuspend, fmt.Sprintf("窗口已过期，%d 份许可已同步过期，现场必须暂停并等待新窗口", assessment.ExpiredClearances)
	case constants.WeatherWindowSafe:
		if assessment.BlockedClearances > 0 || assessment.ReboundClearances > 0 {
			return dto.WindowDecisionSuspend, fmt.Sprintf("窗口虽安全，但仍有 %d 份许可受阻、%d 份待重新确认，相关作业暂停", assessment.BlockedClearances, assessment.ReboundClearances)
		}
		hasApprovedPlan := false
		for _, plan := range plans {
			if plan.Status == "approved" {
				hasApprovedPlan = true
				break
			}
		}
		if !hasApprovedPlan {
			return dto.WindowDecisionSuspend, "窗口安全，但同作业区/关联事项下缺少已批准的系泊方案，暂不具备继续作业条件"
		}
		if assessment.ReadyClearances == 0 {
			return dto.WindowDecisionSuspend, "窗口安全且方案就绪，但尚无当前版本下可放行的安全许可，许可放行后继续"
		}
		return dto.WindowDecisionContinue, "窗口安全、方案已批准且许可为当前窗口版本放行，现场可按方案继续作业"
	default:
		return dto.WindowDecisionSuspend, "窗口仍为预报状态、未确认为安全，现场保持暂停，等待安全窗口确认"
	}
}

func validateWeatherWindowBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
