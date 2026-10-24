package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Payment endpoints: the booking config the customer needs before paying,
// the DOKU callback (signature-gated), and the mock simulation endpoints
// that must not exist outside mock mode.

type handlerFakePaymentRepo struct {
	payments map[string]*entity.Payment
}

func (f *handlerFakePaymentRepo) Create(ctx context.Context, p *entity.Payment) error {
	f.payments[p.InvoiceNumber] = p
	return nil
}
func (f *handlerFakePaymentRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error) {
	for _, p := range f.payments {
		if p.BookingID == bookingID {
			return p, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
}
func (f *handlerFakePaymentRepo) FindByInvoiceNumber(ctx context.Context, invoice string) (*entity.Payment, error) {
	p, ok := f.payments[invoice]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	}
	return p, nil
}
func (f *handlerFakePaymentRepo) UpdateCheckout(ctx context.Context, id uint, checkout repository.PaymentCheckout) error {
	return nil
}
func (f *handlerFakePaymentRepo) UpdateStatus(ctx context.Context, id uint, status string, paidAt *time.Time) error {
	return nil
}
func (f *handlerFakePaymentRepo) UpdateStatusIfPending(ctx context.Context, id uint, status string) (bool, error) {
	return false, nil
}
func (f *handlerFakePaymentRepo) UpdateLastError(ctx context.Context, id uint, lastError string) error {
	return nil
}
func (f *handlerFakePaymentRepo) FindPendingExpired(ctx context.Context, now time.Time) ([]*entity.Payment, error) {
	return nil, nil
}
func (f *handlerFakePaymentRepo) MarkPaidAndActivateBooking(ctx context.Context, invoice string, paidAt time.Time) (*repository.PaymentTransition, error) {
	return &repository.PaymentTransition{}, nil
}
func (f *handlerFakePaymentRepo) MarkExpiredAndExpireBooking(ctx context.Context, invoice string, now time.Time) (*repository.PaymentTransition, error) {
	return &repository.PaymentTransition{}, nil
}
func (f *handlerFakePaymentRepo) MarkRefundPending(ctx context.Context, bookingID uint) error {
	return nil
}

type handlerFakeBookingRepo struct {
	bookings map[uint]*entity.Booking
}

func (f *handlerFakeBookingRepo) Create(ctx context.Context, b *entity.Booking) error { return nil }
func (f *handlerFakeBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return b, nil
}
func (f *handlerFakeBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *handlerFakeBookingRepo) UpdateSchedule(ctx context.Context, id uint, d time.Time, tm string, a string) error {
	return nil
}
func (f *handlerFakeBookingRepo) UpdateCancel(ctx context.Context, id uint, cid uint, by uint) error {
	return nil
}
func (f *handlerFakeBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *handlerFakeBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *handlerFakeBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}

type handlerFakeGateway struct {
	verifyErr error
}

func (g *handlerFakeGateway) Name() string { return "doku" }
func (g *handlerFakeGateway) CreateCheckout(ctx context.Context, req payment.CreateCheckoutRequest) (*payment.CreateCheckoutResult, error) {
	return nil, nil
}
func (g *handlerFakeGateway) VerifyNotification(rawBody []byte, headers map[string]string) error {
	return g.verifyErr
}

func newPaymentHandlerApp(t *testing.T, mode string, gateway payment.Gateway, actor uint, bookings map[uint]*entity.Booking) *fiber.App {
	t.Helper()
	paymentRepo := &handlerFakePaymentRepo{payments: map[string]*entity.Payment{}}
	bookingRepo := &handlerFakeBookingRepo{bookings: bookings}
	uc := usecase.NewPaymentUsecase(paymentRepo, bookingRepo, nil, gateway, mode, 60, "http://localhost:5173")
	h := NewPaymentHandler(uc, 5000)

	app := fiber.New(fiber.Config{ErrorHandler: apierror.GlobalErrorHandler})
	app.Use(func(c fiber.Ctx) error {
		if actor != 0 {
			requestctx.SetUserID(c, actor)
		}
		return c.Next()
	})
	app.Get("/api/v1/booking-config", h.GetConfig)
	app.Get("/api/v1/bookings/:id/payment", h.GetPayment)
	app.Post("/api/v1/payments/doku/webhook", h.Webhook)
	app.Post("/api/v1/payments/mock/:invoice/succeed", h.MockSucceed)
	app.Post("/api/v1/payments/mock/:invoice/expire", h.MockExpire)
	return app
}

func doPaymentRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func TestPaymentHandler_GetConfig_ReturnsPlatformFeeAndQRIS(t *testing.T) {
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{}, 0, nil)

	status, body := doPaymentRequest(t, app, "GET", "/api/v1/booking-config", "")

	require.Equal(t, 200, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(5000), data["platform_fee"])
	assert.Equal(t, float64(60), data["payment_expiry_minutes"])
	assert.Equal(t, []interface{}{"qris"}, data["payment_methods"])
}

func TestPaymentHandler_GetPayment_RequiresAuthentication(t *testing.T) {
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{}, 0, nil)

	status, body := doPaymentRequest(t, app, "GET", "/api/v1/bookings/1/payment", "")

	assert.Equal(t, 401, status)
	assert.Equal(t, "UNAUTHORIZED", body["code"])
}

func TestPaymentHandler_GetPayment_RejectsNonParticipant(t *testing.T) {
	bookings := map[uint]*entity.Booking{
		1: {ID: 1, CustomerID: 10, ProviderID: 20},
	}
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{}, 99, bookings)

	status, body := doPaymentRequest(t, app, "GET", "/api/v1/bookings/1/payment", "")

	assert.Equal(t, 403, status)
	assert.Equal(t, "NOT_PARTICIPANT", body["code"])
}

func TestPaymentHandler_Webhook_RejectsInvalidSignature(t *testing.T) {
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{verifyErr: payment.ErrInvalidSignature}, 0, nil)

	status, body := doPaymentRequest(t, app, "POST", "/api/v1/payments/doku/webhook",
		`{"order":{"invoice_number":"SRB-1-1","amount":10000},"transaction":{"status":"SUCCESS"}}`)

	assert.Equal(t, 401, status)
	assert.Equal(t, "INVALID_SIGNATURE", body["code"])
}

func TestPaymentHandler_Webhook_AcknowledgesUnknownInvoice(t *testing.T) {
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{}, 0, nil)

	status, body := doPaymentRequest(t, app, "POST", "/api/v1/payments/doku/webhook",
		`{"order":{"invoice_number":"UNKNOWN-1","amount":10000},"transaction":{"status":"SUCCESS"}}`)

	assert.Equal(t, 200, status, "an unknown invoice is acknowledged so DOKU stops retrying")
	assert.Equal(t, "IGNORED", body["message"])
}

func TestPaymentHandler_MockEndpoints_AreDisabledOutsideMockMode(t *testing.T) {
	app := newPaymentHandlerApp(t, "sandbox", &handlerFakeGateway{}, 0, nil)

	status, body := doPaymentRequest(t, app, "POST", "/api/v1/payments/mock/SRB-1-1/succeed", "")

	assert.Equal(t, 404, status)
	assert.Equal(t, "NOT_FOUND", body["code"])
}
