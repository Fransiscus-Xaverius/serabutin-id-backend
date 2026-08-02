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

type BidRepository struct {
	db *gorm.DB
}

func NewBidRepository(db *gorm.DB) *BidRepository {
	return &BidRepository{db: db}
}

func (r *BidRepository) Create(ctx context.Context, bid *entity.Bid) error {
	m := &model.BidModel{
		ServiceID:  bid.ServiceID,
		ProviderID: bid.ProviderID,
		Price:      bid.Price,
		Notes:      bid.Notes,
		Status:     bid.Status,
	}

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	bid.ID = m.ID
	bid.CreatedAt = m.CreatedAt
	bid.UpdatedAt = m.UpdatedAt

	return nil
}

func (r *BidRepository) FindByID(ctx context.Context, id uint) (*entity.Bid, error) {
	var m model.BidModel

	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "BID_NOT_FOUND", "bid not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *BidRepository) FindByServiceID(ctx context.Context, serviceID uint) ([]*entity.Bid, error) {
	var models []model.BidModel

	err := r.db.WithContext(ctx).
		Where("service_id = ?", serviceID).
		Order("price ASC").
		Find(&models).Error
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntities(models), nil
}

func (r *BidRepository) FindByProviderID(ctx context.Context, providerID uint, filter repository.BidFilter) ([]*entity.Bid, int64, error) {
	var models []model.BidModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.BidModel{}).Where("provider_id = ?", providerID)

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	offset := (filter.Page - 1) * filter.Limit
	err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(filter.Limit).
		Find(&models).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntities(models), total, nil
}

func (r *BidRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	result := r.db.WithContext(ctx).Model(&model.BidModel{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "BID_NOT_FOUND", "bid not found")
	}
	return nil
}

// AcceptIfPending is the race-safe accept transition used by both the
// manual AcceptBid path and the auto-accept path in CreateBid.
// The WHERE status='pending' guard makes two concurrent accepts safe:
// only the first update touches a row; the loser sees 0 rows affected
// and gets ErrInvalidTransition.
func (r *BidRepository) AcceptIfPending(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).
		Model(&model.BidModel{}).
		Where("id = ? AND status = ?", id, "pending").
		Update("status", "accepted")
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only accept pending bids")
	}
	return nil
}

func (r *BidRepository) RejectOthers(ctx context.Context, serviceID uint, exceptID uint) error {
	result := r.db.WithContext(ctx).Model(&model.BidModel{}).
		Where("service_id = ? AND id != ? AND status = ?", serviceID, exceptID, "pending").
		Update("status", "rejected")
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	return nil
}

func (r *BidRepository) toEntity(m *model.BidModel) *entity.Bid {
	return &entity.Bid{
		ID:         m.ID,
		ServiceID:  m.ServiceID,
		ProviderID: m.ProviderID,
		Price:      m.Price,
		Notes:      m.Notes,
		Status:     m.Status,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func (r *BidRepository) toEntities(models []model.BidModel) []*entity.Bid {
	entities := make([]*entity.Bid, len(models))
	for i := range models {
		entities[i] = r.toEntity(&models[i])
	}
	return entities
}
