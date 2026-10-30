package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type BookingRepository interface {
	// Create returns apperror.ErrDuplicateKey when the booking's idempotency
	// key is already held by another committed booking.
	Create(ctx context.Context, booking *entity.Booking) error
	FindByID(ctx context.Context, id uint) (*entity.Booking, error)
	UpdateStatus(ctx context.Context, id uint, status string) error
	// UpdateSchedule fills the schedule/address of a bid-derived booking that
	// was created without one (nullable columns), before it is accepted.
	UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error
	UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error
	FindByUserID(ctx context.Context, userID uint, role string, filter BookingFilter) ([]*entity.Booking, int64, error)
	// FindByIdempotencyKey returns the booking created with the key, or
	// BOOKING_NOT_FOUND when no booking carries it.
	FindByIdempotencyKey(ctx context.Context, key string) (*entity.Booking, error)
	FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error)
	// ListForAdmin serves the admin bookings queue: optional status,
	// customer, provider, and schedule-date-range filters. Empty fields
	// mean no constraint on that axis.
	ListForAdmin(ctx context.Context, filter AdminBookingFilter) ([]*entity.Booking, int64, error)
}

type BookingFilter struct {
	Status string
	Page   int
	Limit  int
}

// AdminBookingFilter scopes the admin bookings queue. DateFrom/DateTo are
// YYYY-MM-DD bounds matched against schedule_date (inclusive).
type AdminBookingFilter struct {
	Status     string
	CustomerID uint
	ProviderID uint
	DateFrom   string
	DateTo     string
	Page       int
	Limit      int
}
