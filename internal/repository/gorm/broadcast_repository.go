package gormrepo

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type BroadcastRepository struct {
	db *gorm.DB
}

func NewBroadcastRepository(db *gorm.DB) *BroadcastRepository {
	return &BroadcastRepository{db: db}
}

func (r *BroadcastRepository) Create(ctx context.Context, b *entity.Broadcast) error {
	m := &model.BroadcastModel{
		Title:          b.Title,
		Body:           b.Body,
		Audience:       b.Audience,
		Status:         b.Status,
		ScheduledAt:    b.ScheduledAt,
		RecipientCount: b.RecipientCount,
		SentAt:         b.SentAt,
		CreatedBy:      b.CreatedBy,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	b.ID = m.ID
	b.CreatedAt = m.CreatedAt
	b.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *BroadcastRepository) FindByID(ctx context.Context, id uint) (*entity.Broadcast, error) {
	var m model.BroadcastModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "BROADCAST_NOT_FOUND", "broadcast not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return toBroadcastEntity(&m), nil
}

func (r *BroadcastRepository) List(ctx context.Context, page, limit int) ([]*entity.Broadcast, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.BroadcastModel{}).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	var models []model.BroadcastModel
	if err := r.db.WithContext(ctx).Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Broadcast, len(models))
	for i := range models {
		out[i] = toBroadcastEntity(&models[i])
	}
	return out, total, nil
}

// Cancel is a status-CAS write: only scheduled rows move to cancelled.
func (r *BroadcastRepository) Cancel(ctx context.Context, id uint) (*entity.Broadcast, error) {
	res := r.db.WithContext(ctx).Model(&model.BroadcastModel{}).
		Where("id = ? AND status = ?", id, entity.BroadcastStatusScheduled).
		Updates(map[string]interface{}{"status": entity.BroadcastStatusCancelled, "updated_at": time.Now()})
	if res.Error != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, r.cancelFailure(ctx, id)
	}
	return r.FindByID(ctx, id)
}

func (r *BroadcastRepository) cancelFailure(ctx context.Context, id uint) error {
	var m model.BroadcastModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperror.NewDomainError(apperror.ErrNotFound, "BROADCAST_NOT_FOUND", "broadcast not found")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only scheduled broadcasts can be cancelled")
}

// ClaimDue atomically flips due scheduled rows to sending inside one
// transaction (SELECT .. FOR UPDATE SKIP LOCKED), so concurrent pollers —
// or a restart racing the ticker — can never deliver the same blast twice.
func (r *BroadcastRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*entity.Broadcast, error) {
	if limit < 1 {
		limit = 10
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", tx.Error.Error())
	}
	var ids []uint
	if err := tx.Raw(`SELECT id FROM broadcasts WHERE status = ? AND scheduled_at IS NOT NULL AND scheduled_at <= ? ORDER BY scheduled_at ASC LIMIT ? FOR UPDATE SKIP LOCKED`,
		entity.BroadcastStatusScheduled, now, limit).Scan(&ids).Error; err != nil {
		tx.Rollback()
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	if len(ids) == 0 {
		tx.Rollback()
		return nil, nil
	}
	if err := tx.Model(&model.BroadcastModel{}).Where("id IN ?", ids).
		Updates(map[string]interface{}{"status": entity.BroadcastStatusSending, "updated_at": time.Now()}).Error; err != nil {
		tx.Rollback()
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	var models []model.BroadcastModel
	if err := tx.Where("id IN ?", ids).Find(&models).Error; err != nil {
		tx.Rollback()
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	if err := tx.Commit().Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Broadcast, len(models))
	for i := range models {
		out[i] = toBroadcastEntity(&models[i])
	}
	return out, nil
}

// MarkSent is a status-CAS write: only sending rows move to sent.
func (r *BroadcastRepository) MarkSent(ctx context.Context, id uint, recipientCount int64) (*entity.Broadcast, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.BroadcastModel{}).
		Where("id = ? AND status = ?", id, entity.BroadcastStatusSending).
		Updates(map[string]interface{}{"status": entity.BroadcastStatusSent, "recipient_count": recipientCount, "sent_at": now, "updated_at": now})
	if res.Error != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only sending broadcasts can be marked sent")
	}
	return r.FindByID(ctx, id)
}

// Requeue flips sending -> scheduled so a failed delivery is retried by the
// ticker instead of dropped. Best-effort: no error when the row already
// moved on (e.g. cancelled mid-send).
func (r *BroadcastRepository) Requeue(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Model(&model.BroadcastModel{}).
		Where("id = ? AND status = ?", id, entity.BroadcastStatusSending).
		Updates(map[string]interface{}{"status": entity.BroadcastStatusScheduled, "updated_at": time.Now()})
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	return nil
}

// recipientScope narrows to active, non-deleted users in the audience.
func recipientScope(db *gorm.DB, audience string) *gorm.DB {
	q := db.Model(&model.UserModel{}).Where("status = ?", entity.UserStatusActive)
	switch audience {
	case entity.BroadcastAudienceCustomer, entity.BroadcastAudienceProvider:
		q = q.Where("role = ?", audience)
	}
	return q
}

func (r *BroadcastRepository) CountRecipients(ctx context.Context, audience string) (int64, error) {
	var total int64
	if err := recipientScope(r.db.WithContext(ctx), audience).Count(&total).Error; err != nil {
		return 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return total, nil
}

// ListRecipientIDs pages active recipient IDs ascending (keyset on id) so
// fan-out never loads the whole user table into memory.
func (r *BroadcastRepository) ListRecipientIDs(ctx context.Context, audience string, afterID uint, batch int) ([]uint, error) {
	if batch < 1 || batch > 1000 {
		batch = 500
	}
	var ids []uint
	if err := recipientScope(r.db.WithContext(ctx), audience).
		Where("users.id > ?", afterID).
		Order("users.id ASC").Limit(batch).Pluck("users.id", &ids).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return ids, nil
}

func toBroadcastEntity(m *model.BroadcastModel) *entity.Broadcast {
	return &entity.Broadcast{
		ID:             m.ID,
		Title:          m.Title,
		Body:           m.Body,
		Audience:       m.Audience,
		Status:         m.Status,
		ScheduledAt:    m.ScheduledAt,
		RecipientCount: m.RecipientCount,
		SentAt:         m.SentAt,
		CreatedBy:      m.CreatedBy,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

var _ repository.BroadcastRepository = (*BroadcastRepository)(nil)
