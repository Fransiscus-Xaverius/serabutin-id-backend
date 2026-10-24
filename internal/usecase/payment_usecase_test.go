package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
)

// Prepayment state machine: a booking is only released to the provider once
// the money is confirmed, and every notification must be idempotent.

type fakePaymentRepo struct {
	payments  map[uint]*entity.Payment
	bookings  map[uint]*entity.Booking
	byInvoice map[string]*entity.Payment
	nextID    uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{
		payments:  map[uint]*entity.Payment{},
		bookings:  map[uint]*entity.Booking{},
		byInvoice: map[string]*entity.Payment{},
		nextID:    1,
	}
}

func (f *fakePaymentRepo) Create(ctx context.Context, p *entity.Payment) error {
	p.ID = f.nextID
	f.nextID++
	f.payments[p.ID] = p
	f.byInvoice[p.InvoiceNumber] = p
	return nil
}

func (f *fakePaymentRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error) {
	for _, p := range f.payments {
		if p.BookingID == bookingID {
			return p, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
}

func (f *fakePaymentRepo) FindByInvoiceNumber(ctx context.Context, invoice string) (*entity.Payment, error) {
	p, ok := f.byInvoice[invoice]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	}
	return p, nil
}

func (f *fakePaymentRepo) UpdateCheckout(ctx context.Context, id uint, checkout repository.PaymentCheckout) error {
	p := f.payments[id]
	if p == nil {
		return errors.New("payment not found")
	}
	p.PaymentURL = checkout.PaymentURL
	p.ProviderRequestID = checkout.ProviderRequestID
	p.ProviderSessionID = checkout.ProviderSessionID
	p.ExpiresAt = checkout.ExpiresAt
	return nil
}

func (f *fakePaymentRepo) UpdateStatus(ctx context.Context, id uint, status string, paidAt *time.Time) error {
	p := f.payments[id]
	if p == nil {
		return errors.New("payment not found")
	}
	p.Status = status
	if paidAt != nil {
		p.PaidAt = paidAt
	}
	return nil
}

func (f *fakePaymentRepo) UpdateStatusIfPending(ctx context.Context, id uint, status string) (bool, error) {
	p := f.payments[id]
	if p == nil {
		return false, errors.New("payment not found")
	}
	if p.Status != entity.PaymentStatusPending {
		return false, nil
	}
	p.Status = status
	return true, nil
}

func (f *fakePaymentRepo) UpdateLastError(ctx context.Context, id uint, lastError string) error {
	p := f.payments[id]
	if p == nil {
		return errors.New("payment not found")
	}
	p.LastError = lastError
	return nil
}

func (f *fakePaymentRepo) FindPendingExpired(ctx context.Context, now time.Time) ([]*entity.Payment, error) {
	var out []*entity.Payment
	for _, p := range f.payments {
		if p.Status == entity.PaymentStatusPending && p.ExpiresAt != nil && p.ExpiresAt.Before(now) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePaymentRepo) MarkPaidAndActivateBooking(ctx context.Context, invoice string, paidAt time.Time) (*repository.PaymentTransition, error) {
	p, ok := f.byInvoice[invoice]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	}
	transition := &repository.PaymentTransition{Payment: p}
	if p.Status == entity.PaymentStatusPaid {
		return transition, nil
	}
	p.Status = entity.PaymentStatusPaid
	p.PaidAt = &paidAt
	transition.Applied = true

	booking := f.bookings[p.BookingID]
	if booking == nil {
		return transition, nil
	}
	transition.Booking = booking
	if booking.Status == entity.BookingStatusPendingPayment {
		booking.Status = entity.BookingStatusPending
		transition.BookingActivated = true
	} else if booking.Status != entity.BookingStatusPending {
		transition.LatePayment = true
	}
	return transition, nil
}

func (f *fakePaymentRepo) MarkExpiredAndExpireBooking(ctx context.Context, invoice string, now time.Time) (*repository.PaymentTransition, error) {
	p, ok := f.byInvoice[invoice]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	}
	transition := &repository.PaymentTransition{Payment: p}
	if p.Status == entity.PaymentStatusPaid || p.Status == entity.PaymentStatusExpired {
		return transition, nil
	}
	p.Status = entity.PaymentStatusExpired
	transition.Applied = true

	booking := f.bookings[p.BookingID]
	if booking == nil {
		return transition, nil
	}
	transition.Booking = booking
	if booking.Status == entity.BookingStatusPendingPayment {
		booking.Status = entity.BookingStatusExpired
	}
	return transition, nil
}

func (f *fakePaymentRepo) MarkRefundPending(ctx context.Context, bookingID uint) error {
	for _, p := range f.payments {
		if p.BookingID == bookingID && p.Status == entity.PaymentStatusPaid {
			p.Status = entity.PaymentStatusRefundPending
		}
	}
	return nil
}

type fakePaymentBookingRepo struct {
	bookings map[uint]*entity.Booking
}

func (f *fakePaymentBookingRepo) Create(ctx context.Context, b *entity.Booking) error { return nil }
func (f *fakePaymentBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return b, nil
}
func (f *fakePaymentBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	f.bookings[id].Status = status
	return nil
}
func (f *fakePaymentBookingRepo) UpdateSchedule(ctx context.Context, id uint, d time.Time, t string, a string) error {
	return nil
}
func (f *fakePaymentBookingRepo) UpdateCancel(ctx context.Context, id uint, cid uint, by uint) error {
	f.bookings[id].Status = "cancelled"
	return nil
}
func (f *fakePaymentBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *fakePaymentBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *fakePaymentBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}

type fakeGateway struct {
	url      string
	err      error
	requests []payment.CreateCheckoutRequest
}

func (g *fakeGateway) Name() string { return "doku" }
func (g *fakeGateway) CreateCheckout(ctx context.Context, req payment.CreateCheckoutRequest) (*payment.CreateCheckoutResult, error) {
	g.requests = append(g.requests, req)
	if g.err != nil {
		return nil, g.err
	}
	return &payment.CreateCheckoutResult{
		PaymentURL:        g.url,
		ProviderRequestID: "req-1",
		ProviderSessionID: "sess-1",
		ExpiresAt:         time.Now().Add(time.Hour),
	}, nil
}
func (g *fakeGateway) VerifyNotification(rawBody []byte, headers map[string]string) error {
	return nil
}

type recordingNotifier struct {
	notifications []struct {
		userID uint
		typ    string
	}
}

func (n *recordingNotifier) Notify(ctx context.Context, userID uint, typ, title, body, data string) (*entity.Notification, error) {
	n.notifications = append(n.notifications, struct {
		userID uint
		typ    string
	}{userID: userID, typ: typ})
	return &entity.Notification{UserID: userID, Type: typ}, nil
}

func newPaymentTestUC(t *testing.T, gateway payment.Gateway) (*PaymentUsecase, *fakePaymentRepo, *fakePaymentBookingRepo, *recordingNotifier) {
	t.Helper()
	paymentRepo := newFakePaymentRepo()
	bookingRepo := &fakePaymentBookingRepo{bookings: map[uint]*entity.Booking{}}
	// The payment repository transitions the booking row as part of the
	// payment transaction, so both fakes share one booking store.
	paymentRepo.bookings = bookingRepo.bookings
	notifier := &recordingNotifier{}
	uc := NewPaymentUsecase(paymentRepo, bookingRepo, nil, gateway, "sandbox", []string{"qris"}, 60, "http://localhost:5173")
	uc.SetNotifier(notifier)
	return uc, paymentRepo, bookingRepo, notifier
}

func TestPaymentUsecase_CreateCheckout_RecordsPaymentAndCheckoutURL(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, paymentRepo, _, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 42, CustomerID: 7, Total: 125000, Status: entity.BookingStatusPendingPayment}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")

	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, entity.PaymentStatusPending, record.Status)
	assert.Equal(t, int64(125000), record.Amount)
	assert.Equal(t, entity.PaymentChannelQRIS, record.Channel)
	assert.Equal(t, "https://checkout.test/pay/abc", record.PaymentURL)
	assert.NotEmpty(t, record.InvoiceNumber)
	assert.Equal(t, "SRB-42-", record.InvoiceNumber[:7])
	require.Len(t, gateway.requests, 1)
	assert.Equal(t, int64(125000), gateway.requests[0].Amount)
	assert.Contains(t, gateway.requests[0].CallbackURL, "/payment/return?booking_id=42")

	stored, err := paymentRepo.FindByBookingID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.test/pay/abc", stored.PaymentURL)
}

func TestPaymentUsecase_CreateCheckout_GatewayFailure_RecordsFailedPayment(t *testing.T) {
	gateway := &fakeGateway{err: errors.New("doku unreachable")}
	uc, paymentRepo, _, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 43, CustomerID: 7, Total: 90000}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")

	require.Error(t, err)
	require.NotNil(t, record)
	assert.Equal(t, entity.PaymentStatusFailed, record.Status)
	assert.Empty(t, record.PaymentURL)

	stored, findErr := paymentRepo.FindByBookingID(context.Background(), 43)
	require.NoError(t, findErr)
	assert.Equal(t, entity.PaymentStatusFailed, stored.Status)
	assert.Contains(t, stored.LastError, "doku unreachable")
}

func notificationBody(invoice string, amount int64, status string) []byte {
	payload := map[string]interface{}{
		"order": map[string]interface{}{
			"invoice_number": invoice,
			"amount":         amount,
		},
		"transaction": map[string]interface{}{
			"status": status,
		},
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func TestPaymentUsecase_ApplyNotification_SuccessMarksPaidAndReleasesBooking(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, bookingRepo, notifier := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 10, CustomerID: 7, ProviderID: 9, Total: 50000, Status: entity.BookingStatusPendingPayment}
	bookingRepo.bookings[10] = booking
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	transition, err := uc.ApplyNotification(context.Background(),
		notificationBody(record.InvoiceNumber, 50000, "SUCCESS"), nil)

	require.NoError(t, err)
	require.NotNil(t, transition)
	assert.True(t, transition.Applied)
	assert.True(t, transition.BookingActivated)
	assert.Equal(t, entity.BookingStatusPending, booking.Status)
	assert.Equal(t, entity.PaymentStatusPaid, record.Status)

	// The provider only learns about the booking once the money arrived.
	var providerNotified bool
	for _, n := range notifier.notifications {
		if n.userID == 9 {
			providerNotified = true
		}
	}
	assert.True(t, providerNotified, "provider must be notified after payment")
}

func TestPaymentUsecase_ApplyNotification_DuplicateIsIdempotent(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, _, notifier := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 11, CustomerID: 7, ProviderID: 9, Total: 50000, Status: entity.BookingStatusPendingPayment}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)
	body := notificationBody(record.InvoiceNumber, 50000, "SUCCESS")

	_, err = uc.ApplyNotification(context.Background(), body, nil)
	require.NoError(t, err)
	notificationsAfterFirst := len(notifier.notifications)

	transition, err := uc.ApplyNotification(context.Background(), body, nil)
	require.NoError(t, err)
	assert.False(t, transition.Applied, "a duplicate webhook must not re-apply the transition")
	assert.Equal(t, notificationsAfterFirst, len(notifier.notifications), "a duplicate webhook must not notify twice")
}

func TestPaymentUsecase_ApplyNotification_AmountMismatchIsRejected(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, _, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 12, CustomerID: 7, Total: 50000}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	_, err = uc.ApplyNotification(context.Background(),
		notificationBody(record.InvoiceNumber, 1, "SUCCESS"), nil)

	require.Error(t, err)
	assert.Equal(t, entity.PaymentStatusPending, record.Status)
}

func TestPaymentUsecase_ApplyNotification_ExpiredExpiresBooking(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, bookingRepo, notifier := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 13, CustomerID: 7, Total: 50000, Status: entity.BookingStatusPendingPayment}
	bookingRepo.bookings[13] = booking
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	_, err = uc.ApplyNotification(context.Background(),
		notificationBody(record.InvoiceNumber, 50000, "EXPIRED"), nil)

	require.NoError(t, err)
	assert.Equal(t, entity.PaymentStatusExpired, record.Status)
	assert.Equal(t, entity.BookingStatusExpired, booking.Status)
	require.NotEmpty(t, notifier.notifications)
}

func TestPaymentUsecase_ApplyNotification_FailedKeepsBookingPendingPayment(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, bookingRepo, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 14, CustomerID: 7, Total: 50000, Status: entity.BookingStatusPendingPayment}
	bookingRepo.bookings[14] = booking
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	_, err = uc.ApplyNotification(context.Background(),
		notificationBody(record.InvoiceNumber, 50000, "FAILED"), nil)

	require.NoError(t, err)
	assert.Equal(t, entity.PaymentStatusFailed, record.Status)
	assert.Equal(t, entity.BookingStatusPendingPayment, booking.Status)
}

func TestPaymentUsecase_ExpirePending_ExpiresStaleCheckout(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, bookingRepo, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 15, CustomerID: 7, Total: 50000, Status: entity.BookingStatusPendingPayment}
	bookingRepo.bookings[15] = booking
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	stale := time.Now().Add(-time.Minute)
	record.ExpiresAt = &stale

	count, err := uc.ExpirePending(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, entity.PaymentStatusExpired, record.Status)
	assert.Equal(t, entity.BookingStatusExpired, booking.Status)
}

func TestParseNotificationAmount_AcceptsIntegerDecimalAndString(t *testing.T) {
	// DOKU is inconsistent: most channels send an integer, the QRIS
	// notification sends a decimal ("amount": 20000.00), and some channels
	// quote the number.
	cases := map[string]int64{
		`20000`:     20000,
		`20000.00`:  20000,
		`155000.50`: 155001,
		`"150000"`:  150000,
	}
	for raw, expected := range cases {
		amount, err := parseNotificationAmount(json.RawMessage(raw))
		require.NoError(t, err, "raw %s", raw)
		assert.Equal(t, expected, amount, "raw %s", raw)
	}

	_, err := parseNotificationAmount(json.RawMessage(`"not-a-number"`))
	assert.Error(t, err)
}

func TestPaymentUsecase_ApplyNotification_QRISDecimalAmountIsAccepted(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, bookingRepo, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 16, CustomerID: 7, ProviderID: 9, Total: 20000, Status: entity.BookingStatusPendingPayment}
	bookingRepo.bookings[16] = booking
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	// The exact QRIS notification shape from DOKU's documentation.
	body := []byte(`{
  "service": {"id": "QRIS", "name": "QRIS"},
  "acquirer": {"id": "DOKU", "name": "DOKU"},
  "channel": {"id": "QRIS_DOKU", "name": "QRIS-DOKU"},
  "order": {"invoice_number": "` + record.InvoiceNumber + `", "amount": 20000.00},
  "transaction": {"status": "SUCCESS", "date": "2025-06-20T04:26:26Z"}
}`)

	transition, err := uc.ApplyNotification(context.Background(), body, nil)

	require.NoError(t, err, "a decimal amount must not fail the notification")
	require.NotNil(t, transition)
	assert.True(t, transition.Applied)
	assert.Equal(t, entity.BookingStatusPending, booking.Status)
	assert.Equal(t, entity.PaymentStatusPaid, record.Status)
}

func TestPaymentUsecase_MarkRefundPendingForBooking_OnlyMovesPaidPayments(t *testing.T) {
	gateway := &fakeGateway{url: "https://checkout.test/pay/abc"}
	uc, _, _, _ := newPaymentTestUC(t, gateway)

	booking := &entity.Booking{ID: 16, CustomerID: 7, Total: 50000}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	require.NoError(t, uc.MarkRefundPendingForBooking(context.Background(), 16))
	assert.Equal(t, entity.PaymentStatusPending, record.Status, "an unpaid booking has no refund obligation")

	record.Status = entity.PaymentStatusPaid
	require.NoError(t, uc.MarkRefundPendingForBooking(context.Background(), 16))
	assert.Equal(t, entity.PaymentStatusRefundPending, record.Status)
}
