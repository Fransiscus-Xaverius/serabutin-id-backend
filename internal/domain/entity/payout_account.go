package entity

import "time"

const (
	PayoutAccountStatusUnverified = "unverified"
	PayoutAccountStatusVerified   = "verified"
	PayoutAccountStatusFailed     = "failed"
)

// PayoutAccount is the Mitra's bank destination for DOKU payouts. The
// account number and the holder name are encrypted; the bank code, the bank
// name, the last four digits and the fingerprint are not.
type PayoutAccount struct {
	ID                         uint
	UserID                     uint
	BankCode                   string
	BankName                   string
	AccountHolderNameEncrypted string
	AccountNumberEncrypted     string
	PhoneEncrypted             string
	AccountLast4               string
	PhoneLast4                 string
	AccountFingerprint         string
	EncryptionKeyID            string
	Status                     string
	NameMismatch               bool
	ValidatedAt                *time.Time
	LastError                  string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}
