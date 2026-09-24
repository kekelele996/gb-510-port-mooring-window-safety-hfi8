package repository

import (
	"context"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"gorm.io/gorm"
)

// SafetyClearanceRepository owns all persistence operations for 安全许可.
type SafetyClearanceRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.SafetyClearance], error)
	Get(context.Context, uint) (model.SafetyClearance, error)
	FindByWindowScope(context.Context, model.WeatherWindow) ([]model.SafetyClearance, error)
	Create(context.Context, *model.SafetyClearance) error
	Update(context.Context, uint, uint, *model.SafetyClearance) error
	UpdateCascade(context.Context, uint, map[string]interface{}) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type safetyClearanceRepository struct {
	store *Store[model.SafetyClearance]
}

func NewSafetyClearanceRepository(db *gorm.DB) SafetyClearanceRepository {
	return &safetyClearanceRepository{store: NewStore[model.SafetyClearance](db)}
}

func (r *safetyClearanceRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.SafetyClearance], error) {
	return r.store.List(ctx, q)
}
func (r *safetyClearanceRepository) Get(ctx context.Context, id uint) (model.SafetyClearance, error) {
	return r.store.Get(ctx, id)
}

// FindByWindowScope returns clearances in the same 作业区 that are bound to the
// window either directly (related_code = window code) or through the same
// 关联事项 code.
func (r *safetyClearanceRepository) FindByWindowScope(ctx context.Context, window model.WeatherWindow) ([]model.SafetyClearance, error) {
	items := make([]model.SafetyClearance, 0)
	err := r.store.db.WithContext(ctx).
		Where("facility = ? AND (UPPER(related_code) = ? OR UPPER(related_code) = ?)",
			window.Facility, window.Code, window.RelatedCode).
		Order("updated_at DESC, id DESC").Find(&items).Error
	return items, err
}

func (r *safetyClearanceRepository) Create(ctx context.Context, item *model.SafetyClearance) error {
	return r.store.Create(ctx, item)
}
func (r *safetyClearanceRepository) Update(ctx context.Context, id, version uint, item *model.SafetyClearance) error {
	return r.store.Update(ctx, id, version, item)
}

// UpdateCascade performs an unconditional column write for system-driven window
// cascades. The optimistic-lock row itself is bumped via version = version + 1
// so stale client submissions are rejected afterwards.
func (r *safetyClearanceRepository) UpdateCascade(ctx context.Context, id uint, columns map[string]interface{}) error {
	if len(columns) == 0 {
		return nil
	}
	columns["version"] = gorm.Expr("version + 1")
	columns["updated_at"] = time.Now().UTC()
	result := r.store.db.WithContext(ctx).Model(&model.SafetyClearance{}).
		Where("id = ?", id).Updates(columns)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *safetyClearanceRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *safetyClearanceRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
