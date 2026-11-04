package repository

import (
	"context"
	"time"

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
	// FindSubmitted returns up to limit submitted payouts not reconciled since
	// reconciledBefore, for reconciliation.
	FindSubmitted(ctx context.Context, limit int, reconciledBefore time.Time) ([]*entity.Payout, error)
	// FindStalledInquiry returns up to limit payouts in inquiry not updated
	// since updatedBefore, for re-driving.
	FindStalledInquiry(ctx context.Context, limit int, updatedBefore time.Time) ([]*entity.Payout, error)
	// Update writes the mutable columns of a payout. It is deliberately
	// exported: the only holder of this repository is PayoutUsecase, the one
	// package allowed to change a payout's state. Nothing else may call it.
	Update(ctx context.Context, payout *entity.Payout) error
}
