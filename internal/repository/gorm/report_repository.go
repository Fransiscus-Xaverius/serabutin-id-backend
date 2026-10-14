package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type ReportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

func (r *ReportRepository) Create(ctx context.Context, report *entity.Report) error {
	m := r.toModel(report)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	report.ID = m.ID
	report.CreatedAt = m.CreatedAt
	report.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ReportRepository) FindByID(ctx context.Context, id uint) (*entity.Report, error) {
	var m model.ReportModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "REPORT_NOT_FOUND", "report not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return r.toEntity(&m), nil
}

func (r *ReportRepository) ListForAdmin(ctx context.Context, filter repository.ReportFilter) ([]*entity.Report, int64, error) {
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	q := r.db.WithContext(ctx).Model(&model.ReportModel{})
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.TargetType != "" {
		q = q.Where("target_type = ?", filter.TargetType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	var models []model.ReportModel
	if err := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	entities := make([]*entity.Report, len(models))
	for i := range models {
		entities[i] = r.toEntity(&models[i])
	}
	return entities, total, nil
}

func (r *ReportRepository) Resolve(ctx context.Context, id uint, adminID uint, status string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.ReportModel{}).
		Where("id = ? AND status = ?", id, entity.ReportStatusOpen).
		Updates(map[string]interface{}{"status": status, "resolved_by": adminID, "resolved_at": now})
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only open reports can be resolved")
	}
	return nil
}

func (r *ReportRepository) toModel(report *entity.Report) *model.ReportModel {
	return &model.ReportModel{
		ID:         report.ID,
		ReporterID: report.ReporterID,
		TargetType: report.TargetType,
		TargetID:   report.TargetID,
		Reason:     report.Reason,
		Detail:     report.Detail,
		Status:     report.Status,
		ResolvedBy: report.ResolvedBy,
		ResolvedAt: report.ResolvedAt,
	}
}

func (r *ReportRepository) toEntity(m *model.ReportModel) *entity.Report {
	return &entity.Report{
		ID:         m.ID,
		ReporterID: m.ReporterID,
		TargetType: m.TargetType,
		TargetID:   m.TargetID,
		Reason:     m.Reason,
		Detail:     m.Detail,
		Status:     m.Status,
		ResolvedBy: m.ResolvedBy,
		ResolvedAt: m.ResolvedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
