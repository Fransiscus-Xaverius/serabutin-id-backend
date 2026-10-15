package dto

// FinanceSummaryResponse is the money block of the admin finance endpoints:
// GMV/fees over non-cancelled bookings, the per-payment-method GMV split,
// and completed/cancelled counts. Derived only — no ledger in v1.
type FinanceSummaryResponse struct {
	GMV             int64            `json:"gmv"`
	PlatformFees    int64            `json:"platform_fees"`
	ByPaymentMethod map[string]int64 `json:"by_payment_method"`
	CompletedCount  int64            `json:"completed_count"`
	CancelledCount  int64            `json:"cancelled_count"`
}

// FinanceTransactionRow is one booking rendered as a money row.
type FinanceTransactionRow struct {
	ID            uint   `json:"id"`
	CustomerID    uint   `json:"customer_id"`
	ProviderID    uint   `json:"provider_id"`
	ServiceID     uint   `json:"service_id"`
	SubTotal      int64  `json:"sub_total"`
	PlatformFee   int64  `json:"platform_fee"`
	Total         int64  `json:"total"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type FinanceTransactionListResponse struct {
	Data       []FinanceTransactionRow `json:"data"`
	Pagination PaginationMeta          `json:"pagination"`
}

// FinancePayoutRow is one provider's computed earnings for a YYYY-MM
// period. Paid means a manual admin mark exists — never an executed
// transfer in v1.
type FinancePayoutRow struct {
	ProviderID uint   `json:"provider_id"`
	Period     string `json:"period"`
	Gross      int64  `json:"gross"`
	Fee        int64  `json:"fee"`
	Net        int64  `json:"net"`
	Status     string `json:"status"`
}

// FinanceOverviewResponse is the combined GET /admin/finance/overview
// payload: summary aggregates plus bounded transaction/payout lists.
type FinanceOverviewResponse struct {
	Summary      FinanceSummaryResponse  `json:"summary"`
	Transactions []FinanceTransactionRow `json:"transactions"`
	Payouts      []FinancePayoutRow      `json:"payouts"`
}

// FinancePayoutMarkRequest carries the manual paid mark plus its mandatory
// justification. The admin audit middleware persists `reason` automatically.
type FinancePayoutMarkRequest struct {
	ProviderID uint   `json:"provider_id"`
	Period     string `json:"period"`
	Reason     string `json:"reason"`
}

// ReimbursementRow is one claim row: the filing plus the terminal decision
// (decided_by/decided_at stay null while pending).
type ReimbursementRow struct {
	ID          uint    `json:"id"`
	BookingID   uint    `json:"booking_id"`
	FiledBy     uint    `json:"filed_by"`
	Amount      int64   `json:"amount"`
	Reason      string  `json:"reason"`
	EvidenceURL string  `json:"evidence_url,omitempty"`
	Status      string  `json:"status"`
	DecidedBy   *uint   `json:"decided_by,omitempty"`
	DecidedAt   *string `json:"decided_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type ReimbursementListResponse struct {
	Data       []ReimbursementRow `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

// ReimbursementFileRequest is the provider-facing filing payload. Filing
// links the claim to a booking the filer provides.
type ReimbursementFileRequest struct {
	BookingID   uint   `json:"booking_id"`
	Amount      int64  `json:"amount"`
	Reason      string `json:"reason"`
	EvidenceURL string `json:"evidence_url"`
}

// ReimbursementDecideRequest carries the terminal decision plus its
// mandatory justification. The admin audit middleware persists `reason`
// automatically. Approval records the decision only — no money movement.
type ReimbursementDecideRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
