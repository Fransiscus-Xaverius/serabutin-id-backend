package entity

import "time"

// Reimbursement is a provider-filed money-dispute claim linked to one of
// their bookings. Lifecycle: pending -> approved|rejected (terminal).
// Approval in v1 records the decision only — it never moves money
// (payout adjustment stays manual).
type Reimbursement struct {
	ID         uint
	BookingID  uint
	FiledBy    uint
	Amount     int64
	Reason     string
	EvidenceURL string
	Status     string
	DecidedBy  *uint
	DecidedAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Reimbursement statuses.
const (
	ReimbursementPending  = "pending"
	ReimbursementApproved = "approved"
	ReimbursementRejected = "rejected"
)

// Reimbursement decisions accepted by the admin decision endpoint.
const (
	ReimbursementDecisionApproved = "approved"
	ReimbursementDecisionRejected = "rejected"
)

// PayoutMark records an admin's manual "paid" mark for one provider period.
// Payouts themselves are computed from completed bookings; this row only
// prevents recompute confusion in v1 (no payout rail exists).
type PayoutMark struct {
	ID         uint
	ProviderID uint
	Period     string
	MarkedBy   uint
	MarkedAt   time.Time
	CreatedAt  time.Time
}

// Payout statuses.
const (
	PayoutPending = "pending"
	PayoutPaid    = "paid"
)
