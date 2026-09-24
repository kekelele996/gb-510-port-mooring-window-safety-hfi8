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

type SafetyClearanceService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.SafetyClearance], error)
	Get(context.Context, uint) (model.SafetyClearance, error)
	Create(context.Context, dto.CreateSafetyClearance, string, string) (model.SafetyClearance, error)
	Update(context.Context, uint, dto.UpdateSafetyClearance, string, string) (model.SafetyClearance, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.SafetyClearance, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	// BoundWindow resolves the weather window that governs a clearance, first by
	// explicit window code and then by shared facility and related code.
	BoundWindow(context.Context, model.SafetyClearance) (model.WeatherWindow, error)
}

type safetyClearanceService struct {
	repository repository.SafetyClearanceRepository
	windows    repository.WeatherWindowRepository
	security   SecurityService
}

func NewSafetyClearanceService(repo repository.SafetyClearanceRepository, windows repository.WeatherWindowRepository, security SecurityService) SafetyClearanceService {
	return &safetyClearanceService{repository: repo, windows: windows, security: security}
}

func (s *safetyClearanceService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.SafetyClearance], error) {
	return s.repository.List(ctx, query)
}

func (s *safetyClearanceService) Get(ctx context.Context, id uint) (model.SafetyClearance, error) {
	return s.repository.Get(ctx, id)
}

func (s *safetyClearanceService) BoundWindow(ctx context.Context, clearance model.SafetyClearance) (model.WeatherWindow, error) {
	if code := strings.TrimSpace(clearance.WindowCode); code != "" {
		return s.windows.FindByCode(ctx, code)
	}
	if code := strings.TrimSpace(clearance.RelatedCode); code != "" {
		if window, err := s.windows.FindByCode(ctx, code); err == nil {
			return window, nil
		}
	}
	return model.WeatherWindow{}, ErrWindowUnbound
}

func (s *safetyClearanceService) Create(ctx context.Context, input dto.CreateSafetyClearance, actor, requestID string) (model.SafetyClearance, error) {
	if err := validateSafetyClearanceBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.SafetyClearance{}, err
	}
	windowVersion := input.WindowVersion
	if windowVersion == 0 {
		windowVersion = 1
	}
	item := model.SafetyClearance{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.SafetyClearanceInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode:   strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
		WindowCode:    strings.ToUpper(strings.TrimSpace(input.WindowCode)),
		WindowVersion: windowVersion,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("create 安全许可: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "SafetyClearance", item.ID, "", item.Status, "created 安全许可")
	return item, nil
}

func (s *safetyClearanceService) Update(ctx context.Context, id uint, input dto.UpdateSafetyClearance, actor, requestID string) (model.SafetyClearance, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.SafetyClearance{}, err
	}
	if err := validateSafetyClearanceBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.SafetyClearance{}, err
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
	current.WindowCode = strings.ToUpper(strings.TrimSpace(input.WindowCode))
	rebindWindow := input.WindowVersion > 0 && input.WindowVersion != current.WindowVersion
	if rebindWindow {
		// Editing a clearance onto a different window invalidates any prior
		// two-person chain; a fresh submit/review pair is required.
		current.WindowVersion = input.WindowVersion
		current.RebindRequired = false
		current.SubmittedBy = ""
		current.SubmittedAt = nil
		current.ConfirmedBy = ""
		current.ConfirmedAt = nil
	}
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("update 安全许可: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "SafetyClearance", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *safetyClearanceService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.SafetyClearance, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.SafetyClearance{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.SafetyClearanceTransitions, current.Status, target) {
		return model.SafetyClearance{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	if current.Status == string(constants.ClearanceStatePending) && target == string(constants.ClearanceStateCleared) {
		return s.confirmClearance(ctx, current, input, actor, role, requestID)
	}
	if role != model.RoleReviewer && role != model.RoleAdmin {
		return model.SafetyClearance{}, ErrReviewerRequired
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("transition 安全许可: %w", err)
	}
	if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "transition", "SafetyClearance", id, before, target, input.Reason, current.WindowVersion); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *safetyClearanceService) confirmClearance(ctx context.Context, current model.SafetyClearance, input dto.TransitionRequest, actor, role, requestID string) (model.SafetyClearance, error) {
	if input.WindowVersion == 0 {
		return model.SafetyClearance{}, ErrWindowVersion
	}
	// A clearance that a window downgrade rebound to a new version must be
	// re-confirmed by the original submitter before any reviewer can release it.
	if current.RebindRequired {
		if current.SubmittedBy == actor || role == model.RoleAdmin {
			return s.resubmitClearance(ctx, current, input, actor, requestID)
		}
		return model.SafetyClearance{}, ErrRebindRequired
	}
	now := time.Now().UTC()
	if current.SubmittedBy == "" {
		window, err := s.BoundWindow(ctx, current)
		if err != nil {
			return model.SafetyClearance{}, err
		}
		if window.Status != string(constants.WeatherWindowSafe) {
			return model.SafetyClearance{}, fmt.Errorf("%w（窗口当前状态：%s）", ErrWindowUnsafe, window.Status)
		}
		if input.WindowVersion != window.Version {
			return model.SafetyClearance{}, fmt.Errorf("%w（许可 v%d，窗口最新 v%d）", ErrVersionChanged, input.WindowVersion, window.Version)
		}
		current.WindowVersion = window.Version
		current.SubmittedBy = actor
		current.SubmittedAt = &now
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = now
		if err := s.repository.Update(ctx, current.ID, input.ExpectedVersion, &current); err != nil {
			return model.SafetyClearance{}, fmt.Errorf("submit safety confirmation: %w", err)
		}
		if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "clearance_submit", "SafetyClearance", current.ID, current.Status, current.Status, input.Reason, current.WindowVersion); err != nil {
			return model.SafetyClearance{}, fmt.Errorf("persist safety submission audit: %w", err)
		}
		return s.repository.Get(ctx, current.ID)
	}
	if current.WindowVersion != input.WindowVersion {
		return model.SafetyClearance{}, fmt.Errorf("%w（许可 v%d，提交版本 v%d）", ErrVersionChanged, current.WindowVersion, input.WindowVersion)
	}
	window, err := s.BoundWindow(ctx, current)
	if err != nil {
		return model.SafetyClearance{}, err
	}
	if window.Status != string(constants.WeatherWindowSafe) {
		return model.SafetyClearance{}, fmt.Errorf("%w（窗口当前状态：%s）", ErrWindowUnsafe, window.Status)
	}
	if window.Version != current.WindowVersion {
		return model.SafetyClearance{}, fmt.Errorf("%w（许可 v%d，窗口最新 v%d）", ErrVersionChanged, current.WindowVersion, window.Version)
	}
	if current.SubmittedBy == actor {
		return model.SafetyClearance{}, ErrSelfApproval
	}
	if role != model.RoleReviewer && role != model.RoleAdmin {
		return model.SafetyClearance{}, ErrReviewerRequired
	}
	before := current.Status
	current.Status = string(constants.ClearanceStateCleared)
	current.ConfirmedBy = actor
	current.ConfirmedAt = &now
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = now
	if err := s.repository.Update(ctx, current.ID, input.ExpectedVersion, &current); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("confirm safety clearance: %w", err)
	}
	if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "clearance_confirm", "SafetyClearance", current.ID, before, current.Status, input.Reason, current.WindowVersion); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("persist safety confirmation audit: %w", err)
	}
	return s.repository.Get(ctx, current.ID)
}

// resubmitClearance re-confirms a rebound pending clearance against the latest
// safe window version. The original submitter is preserved; only the version
// anchor and re-submit flag change, so the two-person chain can restart.
func (s *safetyClearanceService) resubmitClearance(ctx context.Context, current model.SafetyClearance, input dto.TransitionRequest, actor, requestID string) (model.SafetyClearance, error) {
	window, err := s.BoundWindow(ctx, current)
	if err != nil {
		return model.SafetyClearance{}, err
	}
	if window.Status != string(constants.WeatherWindowSafe) {
		return model.SafetyClearance{}, fmt.Errorf("%w（窗口当前状态：%s）", ErrWindowUnsafe, window.Status)
	}
	if input.WindowVersion != window.Version {
		return model.SafetyClearance{}, fmt.Errorf("%w（提交 v%d，窗口最新 v%d）", ErrVersionChanged, input.WindowVersion, window.Version)
	}
	now := time.Now().UTC()
	current.WindowVersion = window.Version
	current.RebindRequired = false
	current.SubmittedBy = actor
	current.SubmittedAt = &now
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = now
	if err := s.repository.Update(ctx, current.ID, input.ExpectedVersion, &current); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("resubmit safety confirmation: %w", err)
	}
	if err := s.security.AuditWithWindowVersion(ctx, actor, requestID, "clearance_resubmit", "SafetyClearance", current.ID, current.Status, current.Status, input.Reason, current.WindowVersion); err != nil {
		return model.SafetyClearance{}, fmt.Errorf("persist safety resubmission audit: %w", err)
	}
	return s.repository.Get(ctx, current.ID)
}

func (s *safetyClearanceService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "SafetyClearance", id, current.Status, "deleted", "soft deleted 安全许可")
}

func (s *safetyClearanceService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateSafetyClearanceBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
