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
	CustomerName   string
	CustomerAvatar string
}

func (r *ReviewRepository) FindByID(ctx context.Context, id uint) (*entity.Review, error) {
	var m model.ReviewModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "REVIEW_NOT_FOUND", "review not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return r.toEntity(&m), nil
}

func (r *ReviewRepository) FindByServiceID(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, error) {
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("service_id = ? AND hidden = ?", serviceID, false).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	var results []reviewWithCustomer
	offset := (page - 1) * limit
	err := r.db.WithContext(ctx).
		Table("reviews").
		Select("reviews.*, users.name as customer_name, users.avatar_url as customer_avatar").
		Joins("LEFT JOIN users ON users.id = reviews.customer_id").
		Where("reviews.service_id = ? AND reviews.hidden = ?", serviceID, false).
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

	if err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("provider_id = ? AND author_id != ? AND hidden = ?", providerID, providerID, false).Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	var results []reviewWithCustomer
	offset := (page - 1) * limit
	err := r.db.WithContext(ctx).
		Table("reviews").
		Select("reviews.*, users.name as customer_name, users.avatar_url as customer_avatar").
		Joins("LEFT JOIN users ON users.id = reviews.customer_id").
		Where("reviews.provider_id = ? AND reviews.author_id != ? AND reviews.hidden = ?", providerID, providerID, false).
		Order("reviews.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntitiesWithCustomer(results), total, nil
}

// FindReceivedByUserID returns reviews written ABOUT the user by the other party.
func (r *ReviewRepository) FindReceivedByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Review, int64, error) {
	var total int64

	scope := r.db.WithContext(ctx).Model(&model.ReviewModel{}).
		Where("(customer_id = ? OR provider_id = ?) AND author_id != ? AND hidden = ?", userID, userID, userID, false)
	if err := scope.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	var results []reviewWithCustomer
	offset := (page - 1) * limit
	err := r.db.WithContext(ctx).
		Table("reviews").
		Select("reviews.*, users.name as customer_name, users.avatar_url as customer_avatar").
		Joins("LEFT JOIN users ON users.id = reviews.customer_id").
		Where("(reviews.customer_id = ? OR reviews.provider_id = ?) AND reviews.author_id != ? AND reviews.hidden = ?", userID, userID, userID, false).
		Order("reviews.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntitiesWithCustomer(results), total, nil
}

// ListForAdmin serves the admin moderation queue including hidden rows.
// HiddenOnly narrows to the flagged queue; Hidden constrains to one side.
func (r *ReviewRepository) ListForAdmin(ctx context.Context, filter repository.AdminReviewFilter) ([]*entity.Review, int64, error) {
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	q := r.db.WithContext(ctx).Model(&model.ReviewModel{})
	if filter.HiddenOnly {
		q = q.Where("hidden = ?", true)
	} else if filter.Hidden != nil {
		q = q.Where("hidden = ?", *filter.Hidden)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	var models []model.ReviewModel
	if err := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	entities := make([]*entity.Review, len(models))
	for i := range models {
		entities[i] = r.toEntity(&models[i])
	}
	return entities, total, nil
}

// SetHidden flips the moderation flag without deleting the row.
func (r *ReviewRepository) SetHidden(ctx context.Context, id uint, hidden bool) error {
	res := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("id = ?", id).Update("hidden", hidden)
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "REVIEW_NOT_FOUND", "review not found")
	}
	return nil
}

func (r *ReviewRepository) GetServiceStats(ctx context.Context, serviceID uint) (*repository.ServiceReviewStats, error) {
	var stats repository.ServiceReviewStats

	err := r.db.WithContext(ctx).
		Model(&model.ReviewModel{}).
		Select("COALESCE(AVG(rating), 0) as average_rating, COUNT(*) as total_count, COALESCE(AVG(communication_rating), 0) as average_communication, COALESCE(AVG(quality_rating), 0) as average_quality, COALESCE(AVG(value_rating), 0) as average_value").
		Where("service_id = ? AND hidden = ?", serviceID, false).
		Scan(&stats).Error
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return &stats, nil
}

func (r *ReviewRepository) ExistsByBookingAndAuthor(ctx context.Context, bookingID, authorID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ReviewModel{}).Where("booking_id = ? AND author_id = ?", bookingID, authorID).Count(&count).Error
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
		AuthorID:            review.AuthorID,
		AuthorRole:          review.AuthorRole,
		Rating:              review.Rating,
		CommunicationRating: review.CommunicationRating,
		QualityRating:       review.QualityRating,
		ValueRating:         review.ValueRating,
		Comment:             review.Comment,
		Hidden:              review.Hidden,
	}
}

func (r *ReviewRepository) toEntity(m *model.ReviewModel) *entity.Review {
	return &entity.Review{
		ID:                  m.ID,
		BookingID:           m.BookingID,
		ServiceID:           m.ServiceID,
		CustomerID:          m.CustomerID,
		ProviderID:          m.ProviderID,
		AuthorID:            m.AuthorID,
		AuthorRole:          m.AuthorRole,
		Rating:              m.Rating,
		CommunicationRating: m.CommunicationRating,
		QualityRating:       m.QualityRating,
		ValueRating:         m.ValueRating,
		Comment:             m.Comment,
		Hidden:              m.Hidden,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
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
			AuthorID:            res.ReviewModel.AuthorID,
			AuthorRole:          res.ReviewModel.AuthorRole,
			Rating:              res.ReviewModel.Rating,
			CommunicationRating: res.ReviewModel.CommunicationRating,
			QualityRating:       res.ReviewModel.QualityRating,
			ValueRating:         res.ReviewModel.ValueRating,
			Comment:             res.ReviewModel.Comment,
			Hidden:              res.ReviewModel.Hidden,
			CreatedAt:           res.ReviewModel.CreatedAt,
			UpdatedAt:           res.ReviewModel.UpdatedAt,
		}
	}
	return entities
}
