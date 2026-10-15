package gormrepo

import (
	"context"
	"errors"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, n *entity.Notification) error {
	m := &model.NotificationModel{
		UserID: n.UserID,
		Type:   n.Type,
		Title:  n.Title,
		Body:   n.Body,
		Data:   n.Data,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	n.ID = m.ID
	n.CreatedAt = m.CreatedAt
	return nil
}

// broadcastBatchSize bounds one INSERT for fan-out writes (ticket 9).
const broadcastBatchSize = 500

func (r *NotificationRepository) CreateMany(ctx context.Context, notifications []*entity.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	models := make([]model.NotificationModel, len(notifications))
	for i, n := range notifications {
		models[i] = model.NotificationModel{
			UserID: n.UserID,
			Type:   n.Type,
			Title:  n.Title,
			Body:   n.Body,
			Data:   n.Data,
		}
	}
	if err := r.db.WithContext(ctx).CreateInBatches(models, broadcastBatchSize).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	for i := range notifications {
		notifications[i].ID = models[i].ID
		notifications[i].CreatedAt = models[i].CreatedAt
	}
	return nil
}

func (r *NotificationRepository) ListByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Notification, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.NotificationModel{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	offset := (page - 1) * limit
	var models []model.NotificationModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Notification, len(models))
	for i := range models {
		out[i] = toNotificationEntity(&models[i])
	}
	return out, total, nil
}

func (r *NotificationRepository) ListSinceID(ctx context.Context, userID uint, sinceID uint, limit int) ([]*entity.Notification, error) {
	var models []model.NotificationModel
	if err := r.db.WithContext(ctx).Where("user_id = ? AND id > ?", userID, sinceID).Order("id ASC").Limit(limit).Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Notification, len(models))
	for i := range models {
		out[i] = toNotificationEntity(&models[i])
	}
	return out, nil
}

func (r *NotificationRepository) ListAll(ctx context.Context, page, limit int, notifType string) ([]*entity.Notification, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.NotificationModel{})
	if notifType != "" {
		query = query.Where("type = ?", notifType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var models []model.NotificationModel
	if err := query.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Notification, len(models))
	for i := range models {
		out[i] = toNotificationEntity(&models[i])
	}
	return out, total, nil
}

func (r *NotificationRepository) MarkRead(ctx context.Context, userID uint, id uint) error {
	result := r.db.WithContext(ctx).Model(&model.NotificationModel{}).Where("id = ? AND user_id = ?", id, userID).Update("read_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "NOTIFICATION_NOT_FOUND", "notification not found")
	}
	return nil
}

func toNotificationEntity(m *model.NotificationModel) *entity.Notification {
	return &entity.Notification{
		ID:        m.ID,
		UserID:    m.UserID,
		Type:      m.Type,
		Title:     m.Title,
		Body:      m.Body,
		Data:      m.Data,
		ReadAt:    m.ReadAt,
		CreatedAt: m.CreatedAt,
	}
}

var _ repository.NotificationRepository = (*NotificationRepository)(nil)

var _ = errors.Is
