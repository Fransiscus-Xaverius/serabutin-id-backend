package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type BookingRepository interface {
	Create(ctx context.Context, booking *entity.Booking) error
	FindByID(ctx context.Context, id uint) (*entity.Booking, error)
	UpdateStatus(ctx context.Context, id uint, status string) error
	// UpdateSchedule fills the schedule/address of a bid-derived booking that
	// was created without one (nullable columns), before it is accepted.
	UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error
	UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error
	FindByUserID(ctx context.Context, userID uint, role string, filter BookingFilter) ([]*entity.Booking, int64, error)
	FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error)
}

type BookingFilter struct {
	Status string
	Page   int
	Limit  int
}
