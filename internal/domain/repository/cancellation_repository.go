package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type CancellationRepository interface {
	Create(ctx context.Context, c *entity.Cancellation) error
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error)
	List(ctx context.Context, filter CancellationFilter) ([]*entity.Cancellation, int64, error)
}

type CancellationFilter struct {
	BookingID uint
	Role      string
	Page      int
	Limit     int
}
