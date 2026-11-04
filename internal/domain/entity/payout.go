package entity

import "time"

const (
	PayoutStatusPending   = "pending"
	PayoutStatusInquiry   = "inquiry"
	PayoutStatusSubmitted = "submitted"
	PayoutStatusPaid      = "paid"
	PayoutStatusFailed    = "failed"
)

// Payout is one money movement from the platform to a Mitra for a booking.
// There is at most one payout per booking. PartnerReferenceNo is the
// idempotency key sent to DOKU, and ExternalID is the numeric X-EXTERNAL-ID.
// The beneficiary account number is stored encrypted; its fingerprint is the
// encryption-keyed digest, never the plain one.
type Payout struct {
	ID                 uint
	BookingID          uint
	ProviderID         uint
	Amount             int64
	Currency           string
	Channel            string
	Status             string
	PartnerReferenceNo string
	IdempotencyKey     string
	// RequestFingerprint is a reuse detector over the canonical request. It
	// is not a confidentiality control.
	RequestFingerprint string
	ExternalID         string

	DokuInquiryReferenceNo string
	DokuReferenceNo        string
	DokuReferenceNumber    string

	BeneficiaryBankCode                   string
	BeneficiaryAccountHolderNameEncrypted string
	BeneficiaryAccountNumberEncrypted     string
	BeneficiaryLast4                      string
	BeneficiaryFingerprint                string

	Replayed  bool
	LastError string
	PaidAt    *time.Time
	// LastReconciledAt is the last time the worker or the webhook asked DOKU
	// for this payout's status. A recent attempt is not repeated.
	LastReconciledAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
