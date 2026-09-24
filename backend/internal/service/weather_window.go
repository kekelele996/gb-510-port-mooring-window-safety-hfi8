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
	Impact(context.Context, uint) (dto.WindowImpact, error)
	Create(context.Context, dto.CreateWeatherWindow, string, string) (model.WeatherWindow, error)
	Update(context.Context, uint, dto.UpdateWeatherWindow, string, string) (model.WeatherWindow, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.WeatherWindow, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
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

// Impact evaluates which 系泊方案 and 安全许可 share the window's 作业区 and
// 关联事项, and derives an explicit on-site 继续/暂停 conclusion from window state.
func (s *weatherWindowService) Impact(ctx context.Context, id uint) (dto.WindowImpact, error) {
	window, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.WindowImpact{}, err
	}
	plans, err := s.plans.FindByScope(ctx, window.Facility, window.RelatedCode)
	if err != nil {
		return dto.WindowImpact{}, fmt.Errorf("load impacted plans: %w", err)
	}
	clearances, err := s.clearances.FindByWindowScope(ctx, window)
	if err != nil {
		return dto.WindowImpact{}, fmt.Errorf("load impacted clearances: %w", err)
	}

	impact := dto.WindowImpact{
		Window:     window,
		Plans:      make([]dto.ImpactedPlan, 0, len(plans)),
		Clearances: make([]dto.ImpactedClearance, 0, len(clearances)),
	}
	safe := window.Status == "safe"
	if safe {
		impact.Decision = "continue"
		impact.DecisionReason = "窗口处于安全（safe）状态，现场可按已批准方案继续作业。"
	} else {
		impact.Decision = "suspend"
		switch window.Status {
		case "restricted":
			impact.DecisionReason = "窗口已转为受限（restricted），现场暂停靠泊与系泊作业，已放行许可同步过期，待复核许可换绑新窗口版本后方可继续。"
		case "expired":
			impact.DecisionReason = "窗口已过期（expired），现场立即暂停作业，关联已放行许可全部失效，须等待新的安全窗口。"
		default:
			impact.DecisionReason = "窗口尚未确认安全（forecast），现场暂停作业，待窗口评估为安全且许可重新确认后再继续。"
		}
	}

	for _, plan := range plans {
		impact.Plans = append(impact.Plans, dto.ImpactedPlan{
			ID: plan.ID, Code: plan.Code, Name: plan.Name, Status: plan.Status,
			RiskLevel: plan.RiskLevel, Owner: plan.Owner, Version: plan.Version,
			Recommendation: planRecommendation(plan.Status, safe),
		})
		impact.PlanCount++
	}

	for _, clearance := range clearances {
		stale := clearance.WindowVersion != 0 && clearance.WindowVersion != window.Version
		item := dto.ImpactedClearance{
			ID: clearance.ID, Code: clearance.Code, Name: clearance.Name, Status: clearance.Status,
			RiskLevel: clearance.RiskLevel, Owner: clearance.Owner, Version: clearance.Version,
			WindowVersion: clearance.WindowVersion, SubmittedBy: clearance.SubmittedBy,
			ConfirmedBy: clearance.ConfirmedBy, StaleVersion: stale,
			Recommendation: clearanceRecommendation(clearance, window, safe, stale),
		}
		if stale {
			impact.StaleCount++
		}
		if clearance.Status == string(constants.ClearanceStateExpired) {
			impact.ExpiredCount++
		}
		impact.Clearances = append(impact.Clearances, item)
		impact.ClearanceCount++
	}
	return impact, nil
}

func planRecommendation(status string, safe bool) string {
	if safe {
		switch status {
		case "approved":
			return "方案已批准且窗口安全，可继续执行。"
		case "review":
			return "窗口安全但方案仍在评审，完成审批后可继续。"
		case "draft":
			return "窗口安全但方案尚未送审，不具备作业条件。"
		case "superseded":
			return "方案已被替代，请切换到最新版本后再作业。"
		}
		return "窗口安全，请确认方案状态后作业。"
	}
	switch status {
	case "approved":
		return "窗口转差，已批准方案暂停执行，待窗口恢复安全。"
	case "review", "draft":
		return "窗口转差，暂停方案评审推进，恢复安全后再处理。"
	case "superseded":
		return "方案已被替代，暂停并切换到最新版本。"
	}
	return "窗口不安全，方案相关作业暂停。"
}

func clearanceRecommendation(clearance model.SafetyClearance, window model.WeatherWindow, safe, stale bool) string {
	switch clearance.Status {
	case string(constants.ClearanceStateCleared):
		if !safe {
			return "许可已随窗口同步过期，禁止继续作业。"
		}
		if stale {
			return "许可固化的窗口版本与当前版本不一致，放行前须重新确认窗口。"
		}
		return "许可有效且窗口版本一致，可继续作业。"
	case string(constants.ClearanceStatePending):
		if !safe {
			return "窗口不安全，禁止放行；提交保留有效，换绑当前窗口版本后须由独立复核人重新确认。"
		}
		if stale {
			return "待复核许可仍绑定旧窗口版本，旧提交不能直接放行，须按新版本重新确认。"
		}
		if clearance.SubmittedBy == "" {
			return "待提交安全确认。"
		}
		return "已提交待独立复核，窗口版本一致，可由另一复核人放行。"
	case string(constants.ClearanceStateRestricted):
		return "许可处于受限状态，窗口恢复安全并重新确认后再放行。"
	case string(constants.ClearanceStateExpired):
		return "许可已过期，须在新的安全窗口下重新提交与复核。"
	}
	return "请核实许可与窗口状态。"
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
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("transition 风浪窗口: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "WeatherWindow", id, before, target, input.Reason); err != nil {
		return model.WeatherWindow{}, fmt.Errorf("persist transition audit: %w", err)
	}
	// A degraded window (受限/过期) immediately cascades to linked clearances:
	// released clearances expire together; pending submissions keep their
	// submitter but rebind to the new window version so stale releases fail.
	if target == "restricted" || target == "expired" {
		if err := s.cascadeDegradedWindow(ctx, current, before, target, actor, requestID); err != nil {
			return model.WeatherWindow{}, err
		}
	}
	return s.repository.Get(ctx, id)
}

func (s *weatherWindowService) cascadeDegradedWindow(ctx context.Context, window model.WeatherWindow, before, target, actor, requestID string) error {
	clearances, err := s.clearances.FindByWindowScope(ctx, window)
	if err != nil {
		return fmt.Errorf("load linked clearances: %w", err)
	}
	for _, clearance := range clearances {
		switch clearance.Status {
		case string(constants.ClearanceStateCleared):
			if err := s.clearances.UpdateCascade(ctx, clearance.ID, map[string]interface{}{
				"status": string(constants.ClearanceStateExpired),
			}); err != nil {
				return fmt.Errorf("expire linked clearance %s: %w", clearance.Code, err)
			}
			detail := fmt.Sprintf("窗口 %s %s->%s，已放行许可同步过期", window.Code, before, target)
			if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "window_cascade_expire", "SafetyClearance", clearance.ID, string(constants.ClearanceStateCleared), string(constants.ClearanceStateExpired), detail, window.Version); err != nil {
				return fmt.Errorf("persist clearance expiry audit: %w", err)
			}
		case string(constants.ClearanceStatePending):
			// 仅换绑窗口版本：提交人/提交时间原样保留，乐观锁版本随级联 +1，
			// 因此旧版本提交无法直接放行，必须由独立复核人按新版本重新确认。
			if err := s.clearances.UpdateCascade(ctx, clearance.ID, map[string]interface{}{
				"window_version": window.Version,
			}); err != nil {
				return fmt.Errorf("rebind linked clearance %s: %w", clearance.Code, err)
			}
			detail := fmt.Sprintf("窗口 %s 转为 %s（v%d），待复核许可保留提交人并换绑新窗口版本，旧提交不能直接放行", window.Code, target, window.Version)
			if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "window_cascade_rebind", "SafetyClearance", clearance.ID, clearance.Status, string(constants.ClearanceStatePending), detail, window.Version); err != nil {
				return fmt.Errorf("persist clearance rebind audit: %w", err)
			}
		}
	}
	return nil
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

func validateWeatherWindowBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
