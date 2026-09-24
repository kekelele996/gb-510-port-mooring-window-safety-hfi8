package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"gorm.io/gorm"
)

type SafetyClearanceService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.SafetyClearance], error)
	Get(context.Context, uint) (model.SafetyClearance, error)
	Create(context.Context, dto.CreateSafetyClearance, string, string) (model.SafetyClearance, error)
	Update(context.Context, uint, dto.UpdateSafetyClearance, string, string) (model.SafetyClearance, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.SafetyClearance, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
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
	page, err := s.repository.List(ctx, query)
	if err != nil {
		return page, err
	}
	if err := s.attachLinkedWindows(ctx, page.Items); err != nil {
		return page, err
	}
	return page, nil
}

func (s *safetyClearanceService) Get(ctx context.Context, id uint) (model.SafetyClearance, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return item, err
	}
	projected := []model.SafetyClearance{item}
	if err := s.attachLinkedWindows(ctx, projected); err != nil {
		return item, err
	}
	return projected[0], nil
}

// attachLinkedWindows projects each clearance's linked window current state and
// version onto the records. Resolution prefers related_code = window code and
// falls back to the shared 关联事项 within the same 作业区.
func (s *safetyClearanceService) attachLinkedWindows(ctx context.Context, items []model.SafetyClearance) error {
	if len(items) == 0 {
		return nil
	}
	facilities := make([]string, 0, len(items))
	codes := make([]string, 0, len(items)*2)
	seenFacility, seenCode := map[string]bool{}, map[string]bool{}
	for _, item := range items {
		if !seenFacility[item.Facility] {
			seenFacility[item.Facility] = true
			facilities = append(facilities, item.Facility)
		}
		for _, code := range []string{item.RelatedCode} {
			upper := strings.ToUpper(strings.TrimSpace(code))
			if upper != "" && !seenCode[upper] {
				seenCode[upper] = true
				codes = append(codes, upper)
			}
		}
	}
	windows, err := s.windows.FindForClearances(ctx, facilities, codes)
	if err != nil {
		return fmt.Errorf("resolve linked windows: %w", err)
	}
	for index := range items {
		match := matchLinkedWindow(items[index], windows)
		if match == nil {
			continue
		}
		items[index].WindowLinked = true
		items[index].WindowStatus = match.Status
		items[index].WindowCurrentVersion = match.Version
	}
	return nil
}

func matchLinkedWindow(clearance model.SafetyClearance, windows []model.WeatherWindow) *model.WeatherWindow {
	related := strings.ToUpper(strings.TrimSpace(clearance.RelatedCode))
	if related == "" {
		return nil
	}
	// Prefer an explicit window-code link in the same facility.
	for index := range windows {
		window := &windows[index]
		if window.Facility == clearance.Facility && strings.ToUpper(window.Code) == related {
			return window
		}
	}
	// Fall back to the shared 关联事项 code.
	for index := range windows {
		window := &windows[index]
		if window.Facility == clearance.Facility && strings.ToUpper(window.RelatedCode) == related {
			return window
		}
	}
	return nil
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
	if input.WindowVersion > 0 && input.WindowVersion != current.WindowVersion {
		current.WindowVersion = input.WindowVersion
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
	// Every release into `cleared` — including recovery from `restricted` —
	// must pass the same window safety and current-version guard.
	if target == string(constants.ClearanceStateCleared) {
		if input.WindowVersion == 0 {
			return model.SafetyClearance{}, ErrWindowVersion
		}
		if err := s.validateWindowForClearance(ctx, current, input.WindowVersion); err != nil {
			return model.SafetyClearance{}, err
		}
		current.WindowVersion = input.WindowVersion
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
	// Both the initial two-person submission and the final release must be made
	// against an existing, currently safe window at the exact pinned version.
	if err := s.validateWindowForClearance(ctx, current, input.WindowVersion); err != nil {
		return model.SafetyClearance{}, err
	}
	now := time.Now().UTC()
	if current.SubmittedBy == "" {
		current.WindowVersion = input.WindowVersion
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
		// The window is safe and input.WindowVersion is its current version
		// (validated above), so a mismatch means the submission was pinned to an
		// older window version. The independent reviewer explicitly rebinds it;
		// a request still carrying the old version is rejected upstream.
		current.WindowVersion = input.WindowVersion
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

func (s *safetyClearanceService) validateWindowForClearance(ctx context.Context, current model.SafetyClearance, requestedVersion uint) error {
	window, err := s.windows.FindByCode(ctx, current.RelatedCode)
	if err != nil {
		window, err = s.windows.FindByFacilityAndRelatedCode(ctx, current.Facility, current.RelatedCode)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWindowMissing
		}
		return fmt.Errorf("resolve linked weather window: %w", err)
	}
	if window.Status != "safe" {
		return ErrWindowUnsafe
	}
	if requestedVersion != window.Version {
		return fmt.Errorf("%w：许可提交的窗口版本 v%d 与当前 v%d 不一致，请刷新并按新窗口版本重新确认", ErrWindowVersion, requestedVersion, window.Version)
	}
	return nil
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
