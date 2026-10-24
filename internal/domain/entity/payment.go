package entity

import "time"

const (
	PaymentStatusPending       = "pending"
	PaymentStatusFailed        = "failed"
	PaymentStatusPaid          = "paid"
	PaymentStatusExpired       = "expired"
	PaymentStatusRefundPending = "refund_pending"
	PaymentStatusRefunded      = "refunded"

	PaymentChannelQRIS = "qris"

	PaymentProviderDOKU = "doku"
	PaymentProviderMock = "mock"
)

type Payment struct {
	ID                uint
	BookingID         uint
	InvoiceNumber     string
	Amount            int64
	Currency          string
	Channel           string
	Status            string
	Provider          string
	ProviderRequestID string
	ProviderSessionID string
	PaymentURL        string
	ExpiresAt         *time.Time
	PaidAt            *time.Time
	LastError         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
