package entity

import "time"

const (
	BookingStatusPending        = "pending"
	BookingStatusPendingPayment = "pending_payment"
	BookingStatusExpired        = "expired"
)

type Booking struct {
	ID            uint
	CustomerID    uint
	ServiceID     uint
	ProviderID    uint
	ScheduleDate  *time.Time
	ScheduleTime  *string
	Address       *string
	Notes         string
	SubTotal      int64
	PlatformFee   int64
	Total         int64
	PaymentMethod string
	Status        string
	// IdempotencyKey is the Idempotency-Key the client sent with the create
	// request; empty when the client sent none.
	IdempotencyKey     string
	RequestFingerprint string
	BookingAddOns      []BookingAddOn
	Payment            *Payment
	CancellationID     *uint
	CancelledBy        *uint
	CancelledAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type BookingAddOn struct {
	ID        uint
	BookingID uint
	Name      string
	Price     int64
}
