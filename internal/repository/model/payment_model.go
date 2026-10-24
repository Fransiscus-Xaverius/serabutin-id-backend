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
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (PaymentModel) TableName() string {
	return "payments"
}
