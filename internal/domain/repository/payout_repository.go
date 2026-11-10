package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// PayoutReadExternalIDStart is the first X-EXTERNAL-ID of a read-only DOKU call.
// Read ids come from a persisted sequence that starts here. The money-moving
// ids are payoutID*10+step, which stay below this value for every payout id
// under 100,000,000, so the two bands cannot collide.
const PayoutReadExternalIDStart = 1000000000

// PayoutRepository stores payouts. Only the payout usecase may drive a payout:
// its status changes go through Transition, and nothing else may change it.
type PayoutRepository interface {
	// NextReadExternalID returns the next value of the read-only X-EXTERNAL-ID
	// sequence. Values are persisted, monotonic and never repeat.
	NextReadExternalID(ctx context.Context) (string, error)
	// Create stores a new payout. A second payout for the same booking, or a
	// partner reference number already in use, is rejected.
	Create(ctx context.Context, payout *entity.Payout) error
	FindByID(ctx context.Context, id uint) (*entity.Payout, error)
	// FindByBookingID returns the payout of a booking; a booking has at most one.
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payout, error)
	// FindByPartnerReferenceNo returns the payout that carries the key sent to DOKU.
	FindByPartnerReferenceNo(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error)
	// FindByDokuReferenceNo returns the payout that carries DOKU's own transfer
	// reference. Refund notifications identify a payout only by transactionId,
	// which DOKU fills with referenceNo (or, on some versions, referenceNumber).
	FindByDokuReferenceNo(ctx context.Context, dokuReferenceNo string) (*entity.Payout, error)
	// FindSubmitted returns up to limit submitted payouts not reconciled since
	// reconciledBefore, for reconciliation.
	FindSubmitted(ctx context.Context, limit int, reconciledBefore time.Time) ([]*entity.Payout, error)
	// FindStalledInquiry returns up to limit payouts in inquiry not updated
	// since updatedBefore, for re-driving.
	FindStalledInquiry(ctx context.Context, limit int, updatedBefore time.Time) ([]*entity.Payout, error)
	// SumInFlight totals the amounts of the payouts that may already hold money
	// at DOKU: submitted, or in inquiry. The payout excludePayoutID is left out.
	SumInFlight(ctx context.Context, excludePayoutID uint) (int64, error)
	// Transition moves a payout from `from` to `to`, but only while it is still
	// in `from`. It reports false when the row is in another state, so a stale
	// copy cannot move a payout backwards. A from equal to to is a pure guard.
	Transition(ctx context.Context, payoutID uint, from, to string) (bool, error)
	// Update writes every column except status: references, external id,
	// read attempts and timestamps. It never writes status; Transition does.
	Update(ctx context.Context, payout *entity.Payout) error
	// UpdateIfStatus writes the same columns as Update, but only while the row
	// is still in expectedStatus. A stale copy fails instead of overwriting a
	// newer write, such as paid_at or doku_session_id from a concurrent
	// reconciliation. It never writes status.
	UpdateIfStatus(ctx context.Context, payout *entity.Payout, expectedStatus string) error
}
