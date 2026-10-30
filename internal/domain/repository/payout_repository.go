package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type PayoutRepository interface {
	// Create stores a new payout. A second payout for the same booking, or a
	// partner reference number already in use, is rejected.
	Create(ctx context.Context, payout *entity.Payout) error
	FindByID(ctx context.Context, id uint) (*entity.Payout, error)
	// FindByBookingID returns the payout of a booking; a booking has at most one.
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payout, error)
	// FindByPartnerReferenceNo returns the payout that carries the key sent to DOKU.
	FindByPartnerReferenceNo(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error)
	Update(ctx context.Context, payout *entity.Payout) error
}
