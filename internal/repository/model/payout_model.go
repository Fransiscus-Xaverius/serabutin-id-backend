package model

import "time"

type PayoutModel struct {
	ID         uint   `gorm:"primaryKey"`
	BookingID  uint   `gorm:"uniqueIndex;not null"`
	ProviderID uint   `gorm:"index;not null"`
	Amount     int64  `gorm:"not null"`
	Currency   string `gorm:"size:3;default:IDR"`
	Channel    string `gorm:"size:20;not null"`
	Status     string `gorm:"size:20;not null;index"`
	// PartnerReferenceNo and IdempotencyKey are nullable on purpose: a row
	// that has no key stores NULL, and every NULL is distinct for a unique
	// index. A non-nullable column would be filled with '' for every existing
	// row and the unique index could then not be created on a database that
	// already holds payouts.
	PartnerReferenceNo *string `gorm:"size:64;uniqueIndex"`
	IdempotencyKey     *string `gorm:"size:64;uniqueIndex"`
	RequestFingerprint string  `gorm:"size:64"`
	ExternalID         *string `gorm:"size:64;uniqueIndex"`

	DokuInquiryReferenceNo string `gorm:"size:64"`
	DokuSessionID          string `gorm:"size:64"`
	DokuReferenceNo        string `gorm:"size:64;index"`
	DokuReferenceNumber    string `gorm:"size:64;index"`

	BeneficiaryBankCode                   string `gorm:"size:16"`
	BeneficiaryAccountHolderNameEncrypted string `gorm:"type:text"`
	BeneficiaryAccountNumberEncrypted     string `gorm:"type:text"`
	BeneficiaryLast4                      string `gorm:"size:4"`
	BeneficiaryFingerprint                string `gorm:"size:64"`

	Replayed  bool   `gorm:"not null;default:false"`
	LastError string `gorm:"type:text"`
	PaidAt    *time.Time
	// LastReconciledAt is indexed: the reconciliation queries filter on it.
	LastReconciledAt *time.Time `gorm:"index"`
	ReadAttempts     int        `gorm:"not null;default:0"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (PayoutModel) TableName() string {
	return "payouts"
}
