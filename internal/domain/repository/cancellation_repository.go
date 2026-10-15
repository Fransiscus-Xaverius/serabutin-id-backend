package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type CancellationRepository interface {
	Create(ctx context.Context, c *entity.Cancellation) error
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error)
	List(ctx context.Context, filter CancellationFilter) ([]*entity.Cancellation, int64, error)
	// CreateAndApplyCancel persists the cancellation row and flips the
	// booking to cancelled in ONE transaction (F7). The booking update is
	// guarded by a terminal-state predicate, so a concurrent transition
	// surfaces as not-found-or-terminal instead of a half-applied cancel.
	// Terminal states: completed, cancelled, rejected.
	CreateAndApplyCancel(ctx context.Context, c *entity.Cancellation, cancelledBy uint) error
}

type CancellationFilter struct {
	BookingID uint
	Role      string
	Page      int
	Limit     int
}
