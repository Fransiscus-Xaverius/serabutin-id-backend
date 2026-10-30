package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
)

const (
	paymentModeDisabled = "disabled"
	paymentModeMock     = "mock"
)

type PaymentUsecase struct {
	paymentRepo   repository.PaymentRepository
	bookingRepo   repository.BookingRepository
	userRepo      repository.UserRepository
	gateway       payment.Gateway
	notifier      Notifier
	mode          string
	channels      []string
	expiryMinutes int
	appPublicURL  string
	now           func() time.Time
}

func NewPaymentUsecase(
	paymentRepo repository.PaymentRepository,
	bookingRepo repository.BookingRepository,
	userRepo repository.UserRepository,
	gateway payment.Gateway,
	mode string,
	channels []string,
	expiryMinutes int,
	appPublicURL string,
) *PaymentUsecase {
	if expiryMinutes <= 0 {
		expiryMinutes = 60
	}
	if len(channels) == 0 {
		channels = []string{entity.PaymentChannelQRIS}
	}
	return &PaymentUsecase{
		paymentRepo:   paymentRepo,
		bookingRepo:   bookingRepo,
		userRepo:      userRepo,
		gateway:       gateway,
		mode:          strings.ToLower(strings.TrimSpace(mode)),
		channels:      append([]string(nil), channels...),
		expiryMinutes: expiryMinutes,
		appPublicURL:  strings.TrimRight(appPublicURL, "/"),
		now:           time.Now,
	}
}

// EnabledChannels are the channel slugs we currently offer at checkout.
func (uc *PaymentUsecase) EnabledChannels() []string {
	return append([]string(nil), uc.channels...)
}

// IsPrepaidChannel reports whether a channel slug is one of the enabled
// prepayment channels. Bookings on any other method are rejected, so the
// API cannot create an order that is never paid for.
func (uc *PaymentUsecase) IsPrepaidChannel(channel string) bool {
	for _, enabled := range uc.channels {
		if enabled == channel {
			return true
		}
	}
	return false
}

func (uc *PaymentUsecase) SetNotifier(n Notifier) {
	uc.notifier = n
}

func (uc *PaymentUsecase) ProviderMode() string {
	return uc.mode
}

func (uc *PaymentUsecase) MockMode() bool {
	return uc.mode == paymentModeMock
}

func (uc *PaymentUsecase) ExpiryMinutes() int {
	return uc.expiryMinutes
}

// CreateCheckout persists the payment row first, then asks the provider for a
// checkout session. A provider failure still leaves a real failed payment row
// (with last_error) so the API can report the truth to the customer.
func (uc *PaymentUsecase) CreateCheckout(ctx context.Context, booking *entity.Booking, channel string) (*entity.Payment, error) {
	if booking == nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_BOOKING", "booking is required")
	}
	if uc.gateway == nil {
		return nil, fmt.Errorf("payment gateway is not configured")
	}
	if channel == "" {
		channel = entity.PaymentChannelQRIS
	}
	if !uc.IsPrepaidChannel(channel) {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_METHOD_UNAVAILABLE", "payment method is not available")
	}

	expiresAt := uc.now().Add(time.Duration(uc.expiryMinutes) * time.Minute)
	record := &entity.Payment{
		BookingID:     booking.ID,
		InvoiceNumber: fmt.Sprintf("SRB-%d-%d", booking.ID, uc.now().UnixNano()),
		Amount:        booking.Total,
		Currency:      "IDR",
		Channel:       channel,
		Status:        entity.PaymentStatusPending,
		Provider:      uc.providerName(),
		ExpiresAt:     &expiresAt,
	}
	if err := uc.paymentRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	return uc.submitCheckout(ctx, booking, record)
}

// RetryCheckout sends the booking's payment attempt again with the key and
// fingerprint it was first sent with. It never starts a new attempt, so DOKU
// recognises the request and answers with the original checkout rather than
// a second invoice.
func (uc *PaymentUsecase) RetryCheckout(ctx context.Context, bookingID uint, channel string) (*entity.Payment, error) {
	if uc.gateway == nil {
		return nil, fmt.Errorf("payment gateway is not configured")
	}
	if channel == "" {
		channel = entity.PaymentChannelQRIS
	}
	if !uc.IsPrepaidChannel(channel) {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_METHOD_UNAVAILABLE", "payment method is not available")
	}

	stored, err := uc.paymentRepo.FindByBookingID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if stored.Status == entity.PaymentStatusExpired ||
		(stored.Status == entity.PaymentStatusPending && stored.ExpiresAt != nil && !uc.now().Before(*stored.ExpiresAt)) {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_EXPIRED", "payment has expired, create a new booking")
	}
	if stored.Status == entity.PaymentStatusPending && stored.PaymentURL != "" {
		return stored, nil
	}
	if stored.Status != entity.PaymentStatusPending && stored.Status != entity.PaymentStatusFailed {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_NOT_RETRYABLE", "payment cannot be retried")
	}
	if channel != stored.Channel {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_REQUEST_CHANGED", "payment method does not match the pending payment")
	}

	booking, err := uc.bookingRepo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	return uc.submitCheckout(ctx, booking, stored)
}

// submitCheckout keys the attempt, stores the key and fingerprint before the
// gateway is called, and then records the checkout the gateway returns. A
// row that already carries a key is re-sent with that key, and only if the
// request still matches the stored fingerprint.
func (uc *PaymentUsecase) submitCheckout(ctx context.Context, booking *entity.Booking, record *entity.Payment) (*entity.Payment, error) {
	key := idempotency.CollectionKey(booking.ID, record.ID)
	fingerprint := idempotency.Fingerprint(key, idempotency.CanonicalPaymentRequest(
		record.InvoiceNumber, record.Amount, record.Currency, record.Channel, uc.expiryMinutes,
	))
	if record.IdempotencyKey != "" {
		if record.IdempotencyKey != key || record.RequestFingerprint != fingerprint {
			return record, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_REQUEST_CHANGED", "payment request no longer matches the pending payment")
		}
	} else {
		if err := uc.paymentRepo.UpdateIdempotency(ctx, record.ID, key, fingerprint); err != nil {
			return uc.failCheckout(ctx, record, err)
		}
		record.IdempotencyKey = key
		record.RequestFingerprint = fingerprint
	}

	name, email, phone := "Pelanggan", "", ""
	if uc.userRepo != nil {
		customer, err := uc.userRepo.FindByID(ctx, booking.CustomerID)
		if err != nil {
			log.Printf("[PaymentUsecase.submitCheckout] load customer %d: %v", booking.CustomerID, err)
		} else if customer != nil {
			if customer.Name != "" {
				name = customer.Name
			}
			email = customer.Email
			phone = customer.WhatsApp
		}
	}

	result, err := uc.gateway.CreateCheckout(ctx, payment.CreateCheckoutRequest{
		BookingID:     booking.ID,
		CustomerID:    booking.CustomerID,
		InvoiceNumber: record.InvoiceNumber,
		Amount:        record.Amount,
		Currency:      record.Currency,
		CustomerName:  name,
		CustomerEmail: email,
		CustomerPhone: phone,
		CallbackURL:   fmt.Sprintf("%s/payment/return?booking_id=%d", uc.appPublicURL, booking.ID),
		ExpiryMinutes: uc.expiryMinutes,
		Channel:       record.Channel,
		RequestID:     key,
	})
	if err != nil {
		return uc.failCheckout(ctx, record, err)
	}
	if result == nil || result.PaymentURL == "" {
		return uc.failCheckout(ctx, record, fmt.Errorf("payment gateway returned no checkout url"))
	}

	expiresAt := record.ExpiresAt
	if result.ExpiresAt.IsZero() {
		if expiresAt == nil {
			fallback := uc.now().Add(time.Duration(uc.expiryMinutes) * time.Minute)
			expiresAt = &fallback
		}
		result.ExpiresAt = *expiresAt
	}
	record.PaymentURL = result.PaymentURL
	record.ProviderRequestID = result.ProviderRequestID
	record.ProviderSessionID = result.ProviderSessionID
	record.ExpiresAt = &result.ExpiresAt

	if err := uc.paymentRepo.UpdateCheckout(ctx, record.ID, repository.PaymentCheckout{
		PaymentURL:        record.PaymentURL,
		ProviderRequestID: record.ProviderRequestID,
		ProviderSessionID: record.ProviderSessionID,
		ExpiresAt:         record.ExpiresAt,
	}); err != nil {
		return uc.failCheckout(ctx, record, err)
	}

	record.Replayed = result.Replayed
	if err := uc.paymentRepo.MarkReplayed(ctx, record.ID, record.Replayed); err != nil {
		log.Printf("[PaymentUsecase.submitCheckout] save replayed %d: %v", record.ID, err)
	}
	if record.Status == entity.PaymentStatusFailed {
		record.Status = entity.PaymentStatusPending
		if err := uc.paymentRepo.UpdateStatus(ctx, record.ID, record.Status, nil); err != nil {
			log.Printf("[PaymentUsecase.submitCheckout] reopen %d: %v", record.ID, err)
		}
	}

	return record, nil
}

// failCheckout records a failed checkout attempt with its reason, so the API
// can report the truth to the customer, and returns the original error.
func (uc *PaymentUsecase) failCheckout(ctx context.Context, record *entity.Payment, cause error) (*entity.Payment, error) {
	record.Status = entity.PaymentStatusFailed
	record.LastError = cause.Error()
	if err := uc.paymentRepo.UpdateStatus(ctx, record.ID, record.Status, nil); err != nil {
		log.Printf("[PaymentUsecase.failCheckout] mark failed %d: %v", record.ID, err)
	}
	if err := uc.paymentRepo.UpdateLastError(ctx, record.ID, record.LastError); err != nil {
		log.Printf("[PaymentUsecase.failCheckout] save last error %d: %v", record.ID, err)
	}
	return record, cause
}

func (uc *PaymentUsecase) GetForBooking(ctx context.Context, userID, bookingID uint) (*entity.Payment, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if booking.CustomerID != userID && booking.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrNotBookingParticipant, "NOT_PARTICIPANT", "you are not a participant of this booking")
	}

	payment, err := uc.paymentRepo.FindByBookingID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	return payment, nil
}

// ApplyNotification verifies and applies a raw DOKU notification. The
// verification is skipped only in mock mode (the mock endpoints never use
// this path). Unknown invoices return ErrNotFound so the handler can answer
// 200 IGNORED, as DOKU expects.
func (uc *PaymentUsecase) ApplyNotification(ctx context.Context, rawBody []byte, headers map[string]string) (*repository.PaymentTransition, error) {
	if uc.mode != paymentModeMock {
		if uc.gateway == nil {
			return nil, payment.ErrInvalidSignature
		}
		if err := uc.gateway.VerifyNotification(rawBody, headers); err != nil {
			return nil, err
		}
	}

	var payload dokuNotification
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_NOTIFICATION", "invalid notification payload")
	}

	invoice := strings.TrimSpace(payload.Order.InvoiceNumber)
	if invoice == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_NOTIFICATION", "notification missing invoice number")
	}
	amount, err := parseNotificationAmount(payload.Order.Amount)
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_NOTIFICATION", "notification amount is invalid")
	}
	status := strings.ToUpper(strings.TrimSpace(payload.Transaction.Status))
	if status == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_NOTIFICATION", "notification status is missing")
	}

	stored, err := uc.paymentRepo.FindByInvoiceNumber(ctx, invoice)
	if err != nil {
		return nil, err
	}
	if amount != stored.Amount {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_AMOUNT_MISMATCH", "payment amount does not match the booking")
	}

	return uc.applyStatus(ctx, invoice, stored, status)
}

// ApplyMockStatus is the shared state-transition path used by the local
// succeed/expire simulation endpoints. It only works when DOKU_MODE=mock.
func (uc *PaymentUsecase) ApplyMockStatus(ctx context.Context, invoiceNumber, status string) (*entity.Payment, error) {
	if !uc.MockMode() {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "NOT_FOUND", "mock payment endpoints are disabled")
	}
	stored, err := uc.paymentRepo.FindByInvoiceNumber(ctx, invoiceNumber)
	if err != nil {
		return nil, err
	}
	transition, err := uc.applyStatus(ctx, invoiceNumber, stored, strings.ToUpper(strings.TrimSpace(status)))
	if err != nil {
		return nil, err
	}
	if transition != nil && transition.Payment != nil {
		return transition.Payment, nil
	}
	return stored, nil
}

func (uc *PaymentUsecase) applyStatus(ctx context.Context, invoice string, stored *entity.Payment, status string) (*repository.PaymentTransition, error) {
	switch status {
	case "SUCCESS":
		return uc.applyPaid(ctx, invoice)
	case "EXPIRED":
		return uc.applyExpired(ctx, invoice)
	case "FAILED":
		if _, err := uc.paymentRepo.UpdateStatusIfPending(ctx, stored.ID, entity.PaymentStatusFailed); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		// PENDING and unknown statuses are acknowledged without a state change.
		return nil, nil
	}
}

func (uc *PaymentUsecase) applyPaid(ctx context.Context, invoice string) (*repository.PaymentTransition, error) {
	transition, err := uc.paymentRepo.MarkPaidAndActivateBooking(ctx, invoice, uc.now().UTC())
	if err != nil {
		return nil, err
	}
	if transition == nil || !transition.Applied {
		return transition, nil
	}

	if transition.BookingActivated && transition.Booking != nil {
		uc.notifyCustomer(ctx, transition.Booking, "payment.paid",
			"Pembayaran diterima", "Pembayaran diterima. Pesanan menunggu konfirmasi Mitra.")
		uc.notifyProvider(ctx, transition.Booking, "payment.paid",
			"Pesanan baru menunggu konfirmasi", "Pesanan baru menunggu konfirmasi.")
		return transition, nil
	}

	if transition.LatePayment && transition.Booking != nil {
		log.Printf("[PaymentUsecase.applyPaid] late payment for booking %d (status %s)", transition.Booking.ID, transition.Booking.Status)
		uc.notifyCustomer(ctx, transition.Booking, "payment.late",
			"Pembayaran diterima", "Pembayaran diterima, namun pesanan sudah tidak aktif. Hubungi dukungan.")
	}
	return transition, nil
}

func (uc *PaymentUsecase) applyExpired(ctx context.Context, invoice string) (*repository.PaymentTransition, error) {
	transition, err := uc.paymentRepo.MarkExpiredAndExpireBooking(ctx, invoice, uc.now().UTC())
	if err != nil {
		return nil, err
	}
	if transition == nil || !transition.Applied || transition.Booking == nil {
		return transition, nil
	}
	uc.notifyCustomer(ctx, transition.Booking, "payment.expired",
		"Pembayaran kedaluwarsa", "Pembayaran kedaluwarsa. Silakan buat pesanan baru.")
	return transition, nil
}

// ExpirePending is the worker body: expire every pending payment whose due
// date has passed. Per-row failures are logged and skipped.
func (uc *PaymentUsecase) ExpirePending(ctx context.Context) (int, error) {
	payments, err := uc.paymentRepo.FindPendingExpired(ctx, uc.now())
	if err != nil {
		return 0, err
	}

	expired := 0
	for _, stored := range payments {
		transition, err := uc.applyExpired(ctx, stored.InvoiceNumber)
		if err != nil {
			log.Printf("[PaymentUsecase.ExpirePending] invoice %s: %v", stored.InvoiceNumber, err)
			continue
		}
		if transition != nil && transition.Applied {
			expired++
		}
	}
	return expired, nil
}

func (uc *PaymentUsecase) MarkRefundPendingForBooking(ctx context.Context, bookingID uint) error {
	return uc.paymentRepo.MarkRefundPending(ctx, bookingID)
}

func (uc *PaymentUsecase) providerName() string {
	if uc.mode == paymentModeMock {
		return entity.PaymentProviderMock
	}
	return entity.PaymentProviderDOKU
}

func (uc *PaymentUsecase) notifyCustomer(ctx context.Context, booking *entity.Booking, typ, title, body string) {
	if uc.notifier == nil || booking == nil {
		return
	}
	data := fmt.Sprintf(`{"booking_id":%d,"status":%q}`, booking.ID, booking.Status)
	if _, err := uc.notifier.Notify(ctx, booking.CustomerID, typ, title, body, data); err != nil {
		log.Printf("[PaymentUsecase.notify] customer %d: %v", booking.CustomerID, err)
	}
}

func (uc *PaymentUsecase) notifyProvider(ctx context.Context, booking *entity.Booking, typ, title, body string) {
	if uc.notifier == nil || booking == nil {
		return
	}
	data := fmt.Sprintf(`{"booking_id":%d,"status":%q}`, booking.ID, booking.Status)
	if _, err := uc.notifier.Notify(ctx, booking.ProviderID, typ, title, body, data); err != nil {
		log.Printf("[PaymentUsecase.notify] provider %d: %v", booking.ProviderID, err)
	}
}

// StartPaymentExpiryWorker runs ExpirePending every interval until ctx is
// cancelled. Errors are logged; the server never crashes on a worker pass.
func StartPaymentExpiryWorker(ctx context.Context, uc *PaymentUsecase, interval time.Duration) {
	if uc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}

	run := func() {
		count, err := uc.ExpirePending(ctx)
		if err != nil {
			log.Printf("[PaymentExpiryWorker] expire pending payments: %v", err)
			return
		}
		if count > 0 {
			log.Printf("[PaymentExpiryWorker] expired %d payment(s)", count)
		}
	}

	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

type dokuNotification struct {
	Order struct {
		InvoiceNumber string          `json:"invoice_number"`
		Amount        json.RawMessage `json:"amount"`
	} `json:"order"`
	Transaction struct {
		Status string `json:"status"`
	} `json:"transaction"`
}

// parseNotificationAmount reads order.amount from a DOKU notification.
// DOKU is inconsistent here: most channels send an integer, but the QRIS
// notification sends a decimal (e.g. 20000.00), and some channels quote the
// number as a string. Both are accepted; the value is truncated to whole
// IDR.
func parseNotificationAmount(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))
	value = strings.Trim(value, `"`)
	if value == "" || value == "null" {
		return 0, errors.New("missing amount")
	}
	if amount, err := strconv.ParseInt(value, 10, 64); err == nil {
		return amount, nil
	}
	decimal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, errors.New("invalid amount")
	}
	return int64(math.Round(decimal)), nil
}
