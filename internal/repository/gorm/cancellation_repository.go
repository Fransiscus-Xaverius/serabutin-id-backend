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

type CancellationRepository struct {
	db *gorm.DB
}

func NewCancellationRepository(db *gorm.DB) *CancellationRepository {
	return &CancellationRepository{db: db}
}

func (r *CancellationRepository) Create(ctx context.Context, c *entity.Cancellation) error {
	m := &model.CancellationModel{
		BookingID:           c.BookingID,
		CancelledBy:         c.CancelledBy,
		CancelledByRole:     c.CancelledByRole,
		Reason:              c.Reason,
		Detail:              c.Detail,
		FeeCharged:          c.FeeCharged,
		PlatformFeeSnapshot: c.PlatformFeeSnapshot,
		SubtotalSnapshot:    c.SubtotalSnapshot,
		PolicyVersion:       c.PolicyVersion,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	c.ID = m.ID
	c.CreatedAt = m.CreatedAt
	return nil
}

func (r *CancellationRepository) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error) {
	var m model.CancellationModel
	if err := r.db.WithContext(ctx).Where("booking_id = ?", bookingID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "CANCELLATION_NOT_FOUND", "cancellation not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return &entity.Cancellation{
		ID:                  m.ID,
		BookingID:           m.BookingID,
		CancelledBy:         m.CancelledBy,
		CancelledByRole:     m.CancelledByRole,
		Reason:              m.Reason,
		Detail:              m.Detail,
		FeeCharged:          m.FeeCharged,
		PlatformFeeSnapshot: m.PlatformFeeSnapshot,
		SubtotalSnapshot:    m.SubtotalSnapshot,
		PolicyVersion:       m.PolicyVersion,
		CreatedAt:           m.CreatedAt,
	}, nil
}

var _ repository.CancellationRepository = (*CancellationRepository)(nil)

func (r *CancellationRepository) List(ctx context.Context, filter repository.CancellationFilter) ([]*entity.Cancellation, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.CancellationModel{})
	if filter.BookingID != 0 {
		query = query.Where("booking_id = ?", filter.BookingID)
	}
	if filter.Role != "" {
		query = query.Where("cancelled_by_role = ?", filter.Role)
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
	var models []model.CancellationModel
	if err := query.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Cancellation, len(models))
	for i := range models {
		m := &models[i]
		out[i] = &entity.Cancellation{
			ID:                  m.ID,
			BookingID:           m.BookingID,
			CancelledBy:         m.CancelledBy,
			CancelledByRole:     m.CancelledByRole,
			Reason:              m.Reason,
			Detail:              m.Detail,
			FeeCharged:          m.FeeCharged,
			PlatformFeeSnapshot: m.PlatformFeeSnapshot,
			SubtotalSnapshot:    m.SubtotalSnapshot,
			PolicyVersion:       m.PolicyVersion,
			CreatedAt:           m.CreatedAt,
		}
	}
	return out, total, nil
}
