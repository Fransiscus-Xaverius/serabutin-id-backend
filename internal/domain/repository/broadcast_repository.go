package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// BroadcastRepository persists broadcast rows and resolves audiences. The
// recipient helpers query the users table directly (active, non-deleted)
// so audience resolution stays inside one repository; callers paginate by
// ID (afterID) and must never load the whole user table into memory.
type BroadcastRepository interface {
	Create(ctx context.Context, b *entity.Broadcast) error
	FindByID(ctx context.Context, id uint) (*entity.Broadcast, error)
	List(ctx context.Context, page, limit int) ([]*entity.Broadcast, int64, error)
	// Cancel flips scheduled -> cancelled. Any other status is an
	// INVALID_STATUS_TRANSITION domain error.
	Cancel(ctx context.Context, id uint) (*entity.Broadcast, error)
	// ClaimDue atomically flips due scheduled rows to sending (FOR UPDATE
	// SKIP LOCKED) and returns them for delivery. Restart-safe: a claimed
	// row can never be claimed twice.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]*entity.Broadcast, error)
	// MarkSent flips sending -> sent with the recipient snapshot.
	MarkSent(ctx context.Context, id uint, recipientCount int64) (*entity.Broadcast, error)
	// Requeue flips sending -> scheduled so a failed delivery is retried
	// by the ticker instead of dropped.
	Requeue(ctx context.Context, id uint) error
	CountRecipients(ctx context.Context, audience string) (int64, error)
	ListRecipientIDs(ctx context.Context, audience string, afterID uint, batch int) ([]uint, error)
}
