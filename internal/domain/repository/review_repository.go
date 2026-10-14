package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type ReviewRepository interface {
	Create(ctx context.Context, review *entity.Review) error
	FindByID(ctx context.Context, id uint) (*entity.Review, error)
	FindByServiceID(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, error)
	FindByProviderID(ctx context.Context, providerID uint, page, limit int) ([]*entity.Review, int64, error)
	FindReceivedByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Review, int64, error)
	GetServiceStats(ctx context.Context, serviceID uint) (*ServiceReviewStats, error)
	ExistsByBookingAndAuthor(ctx context.Context, bookingID, authorID uint) (bool, error)
	// ListForAdmin serves the admin moderation queue: hidden rows are
	// included (filter Hidden=nil means all). Ordered newest first.
	ListForAdmin(ctx context.Context, filter AdminReviewFilter) ([]*entity.Review, int64, error)
	// SetHidden flips the moderation flag; the row is never deleted.
	SetHidden(ctx context.Context, id uint, hidden bool) error
}

// AdminReviewFilter scopes the admin reviews queue. Hidden=nil means all;
// HiddenOnly=true returns only flagged (hidden) rows.
type AdminReviewFilter struct {
	HiddenOnly bool
	Hidden     *bool
	Page       int
	Limit      int
}

type ServiceReviewStats struct {
	AverageRating        float64
	TotalCount           int64
	AverageCommunication float64
	AverageQuality       float64
	AverageValue         float64
}
