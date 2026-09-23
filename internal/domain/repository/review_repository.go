package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type ReviewRepository interface {
	Create(ctx context.Context, review *entity.Review) error
	FindByServiceID(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, error)
	FindByProviderID(ctx context.Context, providerID uint, page, limit int) ([]*entity.Review, int64, error)
	FindReceivedByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Review, int64, error)
	GetServiceStats(ctx context.Context, serviceID uint) (*ServiceReviewStats, error)
	ExistsByBookingAndAuthor(ctx context.Context, bookingID, authorID uint) (bool, error)
}

type ServiceReviewStats struct {
	AverageRating        float64
	TotalCount           int64
	AverageCommunication float64
	AverageQuality       float64
	AverageValue         float64
}
