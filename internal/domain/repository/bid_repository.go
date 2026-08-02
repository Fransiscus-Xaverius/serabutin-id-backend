package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type BidRepository interface {
	Create(ctx context.Context, bid *entity.Bid) error
	FindByID(ctx context.Context, id uint) (*entity.Bid, error)
	FindByServiceID(ctx context.Context, serviceID uint) ([]*entity.Bid, error)
	FindByProviderID(ctx context.Context, providerID uint, filter BidFilter) ([]*entity.Bid, int64, error)
	UpdateStatus(ctx context.Context, id uint, status string) error
	// AcceptIfPending atomically transitions a bid from pending → accepted.
	// Returns ErrInvalidTransition when the bid is no longer pending
	// (someone else won the race). Caller must treat that as a non-fatal
	// outcome and skip the booking create.
	AcceptIfPending(ctx context.Context, id uint) error
	RejectOthers(ctx context.Context, serviceID uint, exceptID uint) error
}

type BidFilter struct {
	Status string
	Page   int
	Limit  int
}
