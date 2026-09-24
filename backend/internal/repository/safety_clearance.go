package repository

import (
	"context"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"gorm.io/gorm"
)

// CascadeResult reports which clearances were synchronously expired and which
// pending submissions were rebound to a new window version.
type CascadeResult struct {
	Expired []model.SafetyClearance
	Rebound []model.SafetyClearance
}

// SafetyClearanceRepository owns all persistence operations for 安全许可.
type SafetyClearanceRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.SafetyClearance], error)
	Get(context.Context, uint) (model.SafetyClearance, error)
	Create(context.Context, *model.SafetyClearance) error
	Update(context.Context, uint, uint, *model.SafetyClearance) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	ListByWindow(context.Context, model.WeatherWindow) ([]model.SafetyClearance, error)
	// CascadeWindowDowngrade runs the synchronous window impact rules inside one
	// transaction: released clearances expire; submitted pending clearances keep
	// their submitter but rebind to the new window version and require re-submit.
	CascadeWindowDowngrade(ctx context.Context, windowCode, facility, relatedCode string, newVersion uint) (CascadeResult, error)
}

type safetyClearanceRepository struct {
	store *Store[model.SafetyClearance]
	db    *gorm.DB
}

func NewSafetyClearanceRepository(db *gorm.DB) SafetyClearanceRepository {
	return &safetyClearanceRepository{store: NewStore[model.SafetyClearance](db), db: db}
}

func (r *safetyClearanceRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.SafetyClearance], error) {
	return r.store.List(ctx, q)
}
func (r *safetyClearanceRepository) Get(ctx context.Context, id uint) (model.SafetyClearance, error) {
	return r.store.Get(ctx, id)
}
func (r *safetyClearanceRepository) Create(ctx context.Context, item *model.SafetyClearance) error {
	return r.store.Create(ctx, item)
}
func (r *safetyClearanceRepository) Update(ctx context.Context, id, version uint, item *model.SafetyClearance) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *safetyClearanceRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *safetyClearanceRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

// windowScope builds the matching clause shared by impact queries: clearances
// directly bound to the window code, or sharing the window's facility and
// related code (作业区 + 关联事项).
func windowScope(db *gorm.DB, window model.WeatherWindow) *gorm.DB {
	return db.Where(
		"UPPER(window_code) = ? OR (facility = ? AND UPPER(related_code) = ?)",
		strings.ToUpper(window.Code), window.Facility, strings.ToUpper(window.RelatedCode),
	)
}

// ListByWindow returns clearances directly bound to a window code plus those
// sharing the same facility and related code as the window's related matter.
func (r *safetyClearanceRepository) ListByWindow(ctx context.Context, window model.WeatherWindow) ([]model.SafetyClearance, error) {
	items := make([]model.SafetyClearance, 0)
	err := windowScope(r.db.WithContext(ctx).Model(&model.SafetyClearance{}), window).
		Order("updated_at DESC, id DESC").Find(&items).Error
	return items, err
}

func (r *safetyClearanceRepository) CascadeWindowDowngrade(ctx context.Context, windowCode, facility, relatedCode string, newVersion uint) (CascadeResult, error) {
	result := CascadeResult{Expired: []model.SafetyClearance{}, Rebound: []model.SafetyClearance{}}
	window := model.WeatherWindow{}
	window.Code = windowCode
	window.Facility = facility
	window.RelatedCode = relatedCode
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var matched []model.SafetyClearance
		if err := windowScope(tx.Model(&model.SafetyClearance{}), window).Find(&matched).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for i := range matched {
			item := matched[i]
			target := item
			switch item.Status {
			case "cleared":
				target.Status = "expired"
				target.Version = item.Version + 1
				target.UpdatedAt = now
				if err := updateInTx(tx, &target, item.Version); err != nil {
					return err
				}
				result.Expired = append(result.Expired, target)
			case "pending":
				// Unsubmitted pending records pick up the current window on their
				// first submit, so only already-submitted records are rebound.
				if item.SubmittedBy != "" {
					target.WindowVersion = newVersion
					target.RebindRequired = true
					target.Version = item.Version + 1
					target.UpdatedAt = now
					if err := updateInTx(tx, &target, item.Version); err != nil {
						return err
					}
					result.Rebound = append(result.Rebound, target)
				}
			}
		}
		return nil
	})
	return result, err
}

func updateInTx(tx *gorm.DB, item *model.SafetyClearance, expectedVersion uint) error {
	outcome := tx.Model(&model.SafetyClearance{}).
		Where("id = ? AND version = ?", item.ID, expectedVersion).
		Select("*").Omit("id", "code", "created_at", "deleted_at").Updates(item)
	if outcome.Error != nil {
		return outcome.Error
	}
	if outcome.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}
