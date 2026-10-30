package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// PaymentCheckout carries the provider fields learned after the checkout
// session has been created.
type PaymentCheckout struct {
	PaymentURL        string
	ProviderRequestID string
	ProviderSessionID string
	ExpiresAt         *time.Time
}

// PaymentTransition is the outcome of a status transition that touches both
// the payment and its booking. Applied reports whether this call changed the
// payment row; duplicate webhooks therefore have Applied=false.
type PaymentTransition struct {
	Payment          *entity.Payment
	Booking          *entity.Booking
	Applied          bool
	BookingActivated bool
	LatePayment      bool
}

type PaymentRepository interface {
	Create(ctx context.Context, payment *entity.Payment) error
	FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error)
	FindByInvoiceNumber(ctx context.Context, invoiceNumber string) (*entity.Payment, error)
	// FindByIdempotencyKey returns the attempt that carries the key.
	FindByIdempotencyKey(ctx context.Context, key string) (*entity.Payment, error)
	// UpdateIdempotency stores the key and request fingerprint of an attempt.
	// It must run before the provider is called with the key.
	UpdateIdempotency(ctx context.Context, paymentID uint, key, fingerprint string) error
	UpdateCheckout(ctx context.Context, id uint, checkout PaymentCheckout) error
	// MarkReplayed records whether the provider answered the attempt with the
	// original result of an earlier request carrying the same key.
	MarkReplayed(ctx context.Context, paymentID uint, replayed bool) error
	UpdateStatus(ctx context.Context, id uint, status string, paidAt *time.Time) error
	// UpdateStatusIfPending applies status only while the row is still
	// pending; it returns whether the row was changed.
	UpdateStatusIfPending(ctx context.Context, id uint, status string) (bool, error)
	UpdateLastError(ctx context.Context, id uint, lastError string) error
	FindPendingExpired(ctx context.Context, now time.Time) ([]*entity.Payment, error)
	// MarkPaidAndActivateBooking atomically marks the payment paid and, when
	// the booking is still pending_payment, moves the booking to pending.
	// Duplicate calls are idempotent. A late payment never resurrects an
	// expired or cancelled booking.
	MarkPaidAndActivateBooking(ctx context.Context, invoiceNumber string, paidAt time.Time) (*PaymentTransition, error)
	// MarkExpiredAndExpireBooking atomically expires the payment and, when
	// the booking is still pending_payment, expires the booking too.
	MarkExpiredAndExpireBooking(ctx context.Context, invoiceNumber string, now time.Time) (*PaymentTransition, error)
	// MarkRefundPending records the manual refund obligation for a paid
	// payment. It is a no-op for any other status.
	MarkRefundPending(ctx context.Context, bookingID uint) error
}
