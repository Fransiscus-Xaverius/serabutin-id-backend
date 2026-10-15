package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// FinanceSummary is the finance-shaped money rollup for
// GET /admin/finance/summary. All derived from bookings — no ledger exists
// in v1. GMV/Fees sum Total/PlatformFee over non-cancelled bookings in the
// window; refunds are not tracked separately (cancelled_count is the proxy).
type FinanceSummary struct {
	GMV             int64
	PlatformFees    int64
	ByPaymentMethod map[string]int64
	CompletedCount  int64
	CancelledCount  int64
}

// FinanceTransaction is one booking rendered as a money row for
// GET /admin/transactions.
type FinanceTransaction struct {
	ID           uint
	CustomerID   uint
	ProviderID   uint
	ServiceID    uint
	SubTotal     int64
	PlatformFee  int64
	Total        int64
	PaymentMethod string
	Status       string
	CreatedAt    time.Time
}

// FinancePayout is one provider's computed earnings for a YYYY-MM period
// over completed bookings. Status is paid only when a manual payout mark
// exists; otherwise pending. Net = Gross - Fee (provider keeps gross minus
// the platform fee).
type FinancePayout struct {
	ProviderID uint
	Period     string
	Gross      int64
	Fee        int64
	Net        int64
	Status     string
}

// FinanceWindow bounds money reads by bookings.created_at (inclusive).
// Zero From/To means unbounded on that side.
type FinanceWindow struct {
	From time.Time
	To   time.Time
}

// TransactionFilter scopes GET /admin/transactions. Empty Status /
// PaymentMethod mean no constraint on that axis.
type TransactionFilter struct {
	Status        string
	PaymentMethod string
	Window        FinanceWindow
	Page          int
	Limit         int
}

// FinanceRepository is the read-only aggregation port behind the admin
// finance endpoints. Implementations must use GROUP BY / SUM queries —
// never full-table loads.
type FinanceRepository interface {
	Summary(ctx context.Context, window FinanceWindow) (FinanceSummary, error)
	Transactions(ctx context.Context, filter TransactionFilter) ([]FinanceTransaction, int64, error)
	Payouts(ctx context.Context, period string) ([]FinancePayout, error)
	MarkPayoutPaid(ctx context.Context, mark *entity.PayoutMark) error
}

// ReimbursementFilter scopes GET /admin/reimbursements. Empty Status means
// all.
type ReimbursementFilter struct {
	Status string
	Page   int
	Limit  int
}

// ReimbursementRepository persists provider-filed reimbursement claims.
// Lifecycle enforcement (terminal states) lives in the usecase.
type ReimbursementRepository interface {
	Create(ctx context.Context, r *entity.Reimbursement) error
	FindByID(ctx context.Context, id uint) (*entity.Reimbursement, error)
	List(ctx context.Context, filter ReimbursementFilter) ([]*entity.Reimbursement, int64, error)
	Decide(ctx context.Context, id uint, decidedBy uint, status string) error
	// FindPendingByBooking returns the open (pending) claim for a booking,
	// or a NotFound domain error when none exists. It backs the F11
	// one-pending-claim-per-booking rule.
	FindPendingByBooking(ctx context.Context, bookingID uint) (*entity.Reimbursement, error)
}
