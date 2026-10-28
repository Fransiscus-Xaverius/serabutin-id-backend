package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type PayoutAccountRepository interface {
	// Upsert creates or replaces the payout destination of a user.
	Upsert(ctx context.Context, account *entity.PayoutAccount) error
	FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error)
	// FindByFingerprint locates a destination by its keyed digest, so a
	// duplicate account can be detected without decrypting.
	FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error)
}
