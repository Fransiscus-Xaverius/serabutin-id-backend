package gormrepo

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type AuditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, entry *entity.AuditLog) error {
	m := &model.AuditLogModel{
		ActorUserID: entry.ActorUserID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		Reason:      entry.Reason,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	entry.ID = m.ID
	entry.CreatedAt = m.CreatedAt
	return nil
}

func (r *AuditLogRepository) List(ctx context.Context, filter repository.AuditLogFilter) ([]*entity.AuditLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.AuditLogModel{})
	if filter.ActorUserID != 0 {
		query = query.Where("actor_user_id = ?", filter.ActorUserID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var models []model.AuditLogModel
	if err := query.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.AuditLog, len(models))
	for i := range models {
		m := &models[i]
		out[i] = &entity.AuditLog{
			ID:          m.ID,
			ActorUserID: m.ActorUserID,
			Action:      m.Action,
			TargetType:  m.TargetType,
			TargetID:    m.TargetID,
			Reason:      m.Reason,
			CreatedAt:   m.CreatedAt,
		}
	}
	return out, total, nil
}

var _ repository.AuditLogRepository = (*AuditLogRepository)(nil)
