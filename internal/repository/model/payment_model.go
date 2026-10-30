package model

import "time"

type PaymentModel struct {
	ID                uint   `gorm:"primaryKey"`
	BookingID         uint   `gorm:"uniqueIndex;not null"`
	InvoiceNumber     string `gorm:"size:64;uniqueIndex;not null"`
	Amount            int64  `gorm:"not null"`
	Currency          string `gorm:"size:3;default:IDR"`
	Channel           string `gorm:"size:20;not null"`
	Status            string `gorm:"size:20;not null;index"`
	Provider          string `gorm:"size:20;not null"`
	ProviderRequestID string `gorm:"size:64"`
	ProviderSessionID string `gorm:"size:128"`
	PaymentURL        string `gorm:"type:text"`
	ExpiresAt         *time.Time
	PaidAt            *time.Time
	LastError         string `gorm:"type:text"`
	// IdempotencyKey is nullable on purpose: a row that has no key stores
	// NULL, and every NULL is distinct for a unique index. A non-nullable
	// column would be filled with '' for every existing row and the unique
	// index could then not be created on a database that already holds
	// payments.
	IdempotencyKey     *string `gorm:"size:64;uniqueIndex"`
	RequestFingerprint string  `gorm:"size:64"`
	Replayed           bool    `gorm:"not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (PaymentModel) TableName() string {
	return "payments"
}
