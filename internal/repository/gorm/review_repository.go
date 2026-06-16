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

type ReviewRepository struct {
	db *gorm.DB
}

func NewReviewRepository(db *gorm.DB) *ReviewRepository {
	return &ReviewRepository{db: db}
}

func (r *ReviewRepository) Create(ctx context.Context, review *entity.Review) error {
	m := r.toModel(review)

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperror.NewDomainError(apperror.ErrReviewAlreadyExists, "REVIEW_EXISTS", "review already exists for this booking")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	review.ID = m.ID
	review.CreatedAt = m.CreatedAt
	review.UpdatedAt = m.UpdatedAt

	return nil
}

type reviewWithCustomer struct {
	model.ReviewModel
	CustomerName  string
	CustomerAvatar string
}

func (r *ReviewRepository) FindByServiceID(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, error) {
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("service_id = ?", serviceID).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	var results []reviewWithCustomer
	offset := (page - 1) * limit
	err := r.db.WithContext(ctx).
		Table("reviews").
		Select("reviews.*, users.name as customer_name, users.avatar_url as customer_avatar").
		Joins("LEFT JOIN users ON users.id = reviews.customer_id").
		Where("reviews.service_id = ?", serviceID).
		Order("reviews.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntitiesWithCustomer(results), total, nil
}

func (r *ReviewRepository) FindByProviderID(ctx context.Context, providerID uint, page, limit int) ([]*entity.Review, int64, error) {
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("provider_id = ?", providerID).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	var results []reviewWithCustomer
	offset := (page - 1) * limit
	err := r.db.WithContext(ctx).
		Table("reviews").
		Select("reviews.*, users.name as customer_name, users.avatar_url as customer_avatar").
		Joins("LEFT JOIN users ON users.id = reviews.customer_id").
		Where("reviews.provider_id = ?", providerID).
		Order("reviews.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntitiesWithCustomer(results), total, nil
}

func (r *ReviewRepository) GetServiceStats(ctx context.Context, serviceID uint) (*repository.ServiceReviewStats, error) {
	var stats repository.ServiceReviewStats

	err := r.db.WithContext(ctx).
		Model(&model.ReviewModel{}).
		Select("COALESCE(AVG(rating), 0) as average_rating, COUNT(*) as total_count, COALESCE(AVG(communication_rating), 0) as average_communication, COALESCE(AVG(quality_rating), 0) as average_quality, COALESCE(AVG(value_rating), 0) as average_value").
		Where("service_id = ?", serviceID).
		Scan(&stats).Error
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return &stats, nil
}

func (r *ReviewRepository) ExistsByBookingID(ctx context.Context, bookingID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("booking_id = ?", bookingID).Count(&count).Error
	if err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return count > 0, nil
}

func (r *ReviewRepository) toModel(review *entity.Review) *model.ReviewModel {
	return &model.ReviewModel{
		ID:                  review.ID,
		BookingID:           review.BookingID,
		ServiceID:           review.ServiceID,
		CustomerID:          review.CustomerID,
		ProviderID:          review.ProviderID,
		Rating:              review.Rating,
		CommunicationRating: review.CommunicationRating,
		QualityRating:       review.QualityRating,
		ValueRating:         review.ValueRating,
		Comment:             review.Comment,
	}
}

func (r *ReviewRepository) toEntitiesWithCustomer(results []reviewWithCustomer) []*entity.Review {
	entities := make([]*entity.Review, len(results))
	for i, res := range results {
		entities[i] = &entity.Review{
			ID:                  res.ReviewModel.ID,
			BookingID:           res.ReviewModel.BookingID,
			ServiceID:           res.ReviewModel.ServiceID,
			CustomerID:          res.ReviewModel.CustomerID,
			ProviderID:          res.ReviewModel.ProviderID,
			Rating:              res.ReviewModel.Rating,
			CommunicationRating: res.ReviewModel.CommunicationRating,
			QualityRating:       res.ReviewModel.QualityRating,
			ValueRating:         res.ReviewModel.ValueRating,
			Comment:             res.ReviewModel.Comment,
			CreatedAt:           res.ReviewModel.CreatedAt,
			UpdatedAt:           res.ReviewModel.UpdatedAt,
		}
	}
	return entities
}
