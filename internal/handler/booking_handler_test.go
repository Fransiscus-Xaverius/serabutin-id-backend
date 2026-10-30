package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Booking creation honours the Idempotency-Key header: a repeated submit
// answers with the booking it already created instead of a second one.

type idemBookingRepo struct {
	bookings map[uint]*entity.Booking
}

func (f *idemBookingRepo) Create(ctx context.Context, b *entity.Booking) error {
	b.ID = uint(len(f.bookings) + 1)
	f.bookings[b.ID] = b
	return nil
}
func (f *idemBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return b, nil
}
func (f *idemBookingRepo) FindByIdempotencyKey(ctx context.Context, key string) (*entity.Booking, error) {
	for _, b := range f.bookings {
		if b.IdempotencyKey == key {
			return b, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
}
func (f *idemBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *idemBookingRepo) UpdateSchedule(ctx context.Context, id uint, d time.Time, tm string, a string) error {
	return nil
}
func (f *idemBookingRepo) UpdateCancel(ctx context.Context, id uint, cid uint, by uint) error {
	return nil
}
func (f *idemBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *idemBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *idemBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}

type idemPaymentInitiator struct{}

func (idemPaymentInitiator) CreateCheckout(ctx context.Context, booking *entity.Booking, channel string) (*entity.Payment, error) {
	return &entity.Payment{ID: 1, Status: entity.PaymentStatusPending, Channel: channel, PaymentURL: "https://pay.example/1"}, nil
}
func (idemPaymentInitiator) RetryCheckout(ctx context.Context, bookingID uint, channel string) (*entity.Payment, error) {
	return &entity.Payment{ID: 1, Status: entity.PaymentStatusPending, Channel: channel, PaymentURL: "https://pay.example/1"}, nil
}
func (idemPaymentInitiator) IsPrepaidChannel(channel string) bool {
	return channel == entity.PaymentChannelQRIS
}

func newIdemBookingApp(t *testing.T, actor uint, bookingRepo *idemBookingRepo) *fiber.App {
	t.Helper()
	serviceRepo := &mockServiceRepo{}
	serviceRepo.On("FindByID", mock.Anything, mock.Anything).Return(&entity.Service{
		ID: 5, ProviderID: 20, Title: "Bersihkan Rumah", BasePrice: 100000, Status: "active",
	}, nil)
	userRepo := &mockUserRepo{}
	userRepo.On("FindByID", mock.Anything, mock.Anything).Return(&entity.User{ID: 10, Name: "Budi"}, nil)

	bookingUC := usecase.NewBookingUsecase(bookingRepo, serviceRepo, userRepo, 5000, 5000, 7000, nil)
	bookingUC.SetPaymentInitiator(idemPaymentInitiator{})
	h := NewBookingHandler(
		bookingUC,
		usecase.NewProfileUsecase(userRepo),
		usecase.NewServiceUsecase(serviceRepo, userRepo, nil),
	)

	app := fiber.New(fiber.Config{ErrorHandler: apierror.GlobalErrorHandler})
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, actor)
		return c.Next()
	})
	app.Post("/api/v1/bookings", h.Create)
	app.Post("/api/v1/bookings/:id/payment/retry", h.RetryPayment)
	return app
}

func doIdemBookingRequest(t *testing.T, app *fiber.App, url, body string, headers map[string]string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func idemBookingBody(address string) string {
	return `{"service_id":5,"schedule_date":"2026-11-02","schedule_time":"09:30","address":"` + address + `","payment_method":"qris"}`
}

func bookingIDOf(t *testing.T, body map[string]interface{}) uint {
	t.Helper()
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok, "response must carry a data object: %v", body)
	id, ok := data["id"].(float64)
	require.True(t, ok, "booking must carry an id: %v", data)
	return uint(id)
}

func TestBookingHandler_Create_TwiceWithTheSameIdempotencyKey_ReturnsTheSameBooking(t *testing.T) {
	bookingRepo := &idemBookingRepo{bookings: map[uint]*entity.Booking{}}
	app := newIdemBookingApp(t, 10, bookingRepo)
	headers := map[string]string{"Idempotency-Key": "abc"}

	firstStatus, first := doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), headers)
	secondStatus, second := doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), headers)

	require.Equal(t, 201, firstStatus)
	require.Equal(t, 201, secondStatus)
	assert.Equal(t, bookingIDOf(t, first), bookingIDOf(t, second), "the repeated submit must return the same booking")
	assert.Len(t, bookingRepo.bookings, 1, "the repeated submit must not create a second booking")
}

func TestBookingHandler_Create_KeyReusedForAnotherRequest_Returns409(t *testing.T) {
	bookingRepo := &idemBookingRepo{bookings: map[uint]*entity.Booking{}}
	app := newIdemBookingApp(t, 10, bookingRepo)
	headers := map[string]string{"Idempotency-Key": "abc"}

	_, _ = doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), headers)
	status, body := doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Lain 2"), headers)

	assert.Equal(t, 409, status)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", body["code"])
	assert.Len(t, bookingRepo.bookings, 1)
}

func TestBookingHandler_Create_OverlongIdempotencyKeyIsIgnored(t *testing.T) {
	bookingRepo := &idemBookingRepo{bookings: map[uint]*entity.Booking{}}
	app := newIdemBookingApp(t, 10, bookingRepo)
	headers := map[string]string{"Idempotency-Key": strings.Repeat("k", 65)}

	_, _ = doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), headers)
	_, _ = doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), headers)

	assert.Len(t, bookingRepo.bookings, 2, "a key over 64 characters is ignored, so no guarantee applies")
	for _, b := range bookingRepo.bookings {
		assert.Empty(t, b.IdempotencyKey)
	}
}

func TestBookingHandler_RetryPayment_RetriesTheCheckoutOfTheSameBooking(t *testing.T) {
	bookingRepo := &idemBookingRepo{bookings: map[uint]*entity.Booking{}}
	app := newIdemBookingApp(t, 10, bookingRepo)
	_, created := doIdemBookingRequest(t, app, "/api/v1/bookings", idemBookingBody("Jl. Merdeka 1"), map[string]string{"Idempotency-Key": "abc"})
	bookingID := bookingIDOf(t, created)

	status, retried := doIdemBookingRequest(t, app, "/api/v1/bookings/"+strconv.FormatUint(uint64(bookingID), 10)+"/payment/retry", "", nil)

	require.Equal(t, 200, status)
	assert.Equal(t, bookingID, bookingIDOf(t, retried), "a retry answers with the same booking")
	assert.Len(t, bookingRepo.bookings, 1, "a retry never creates a booking")
}

// idemCheckoutGateway answers every checkout with a URL and records the
// request id each call carried.
type idemCheckoutGateway struct {
	url        string
	requestIDs []string
}

func (g *idemCheckoutGateway) Name() string { return "doku" }
func (g *idemCheckoutGateway) CreateCheckout(ctx context.Context, req payment.CreateCheckoutRequest) (*payment.CreateCheckoutResult, error) {
	g.requestIDs = append(g.requestIDs, req.RequestID)
	return &payment.CreateCheckoutResult{PaymentURL: g.url, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (g *idemCheckoutGateway) VerifyNotification(rawBody []byte, headers map[string]string) error {
	return nil
}

// The in-flight contract: a repeat can return the booking while its payment
// row exists but has no checkout URL yet. The client then asks the retry
// endpoint for the checkout, and that must produce the URL. It reuses the
// stored key, so the provider answers with the original checkout.
func TestBookingHandler_RetryPayment_BookingWhosePaymentHasNoURLYet_ReturnsTheCheckoutURL(t *testing.T) {
	bookingRepo := &idemBookingRepo{bookings: map[uint]*entity.Booking{}}
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20, Status: entity.BookingStatusPendingPayment,
		Total: 130000, PaymentMethod: entity.PaymentChannelQRIS,
	}
	paymentRepo := &handlerFakePaymentRepo{payments: map[string]*entity.Payment{}}
	const invoice = "SRB-1-1"
	const key = "SRB-1-1"
	paymentRepo.payments[invoice] = &entity.Payment{
		ID: 1, BookingID: 1, InvoiceNumber: invoice, Amount: 130000, Currency: "IDR",
		Channel: entity.PaymentChannelQRIS, Status: entity.PaymentStatusPending,
		Provider: entity.PaymentProviderDOKU, ExpiresAt: timePtr(time.Now().Add(time.Hour)),
		IdempotencyKey:     key,
		RequestFingerprint: idempotency.Fingerprint(idempotency.CanonicalPaymentRequest(invoice, 130000, "IDR", entity.PaymentChannelQRIS, 60)),
	}
	gateway := &idemCheckoutGateway{url: "https://checkout.example/pay/1"}
	paymentUC := usecase.NewPaymentUsecase(paymentRepo, bookingRepo, nil, gateway, "sandbox", []string{entity.PaymentChannelQRIS}, 60, "http://localhost:5173")

	serviceRepo := &mockServiceRepo{}
	serviceRepo.On("FindByID", mock.Anything, mock.Anything).Return(&entity.Service{ID: 5, ProviderID: 20, Title: "Bersihkan Rumah", Status: "active"}, nil)
	userRepo := &mockUserRepo{}
	userRepo.On("FindByID", mock.Anything, mock.Anything).Return(&entity.User{ID: 10, Name: "Budi"}, nil)
	bookingUC := usecase.NewBookingUsecase(bookingRepo, serviceRepo, userRepo, 5000, 5000, 7000, nil)
	bookingUC.SetPaymentInitiator(paymentUC)
	h := NewBookingHandler(bookingUC, usecase.NewProfileUsecase(userRepo), usecase.NewServiceUsecase(serviceRepo, userRepo, nil))
	app := fiber.New(fiber.Config{ErrorHandler: apierror.GlobalErrorHandler})
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 10)
		return c.Next()
	})
	app.Post("/api/v1/bookings/:id/payment/retry", h.RetryPayment)

	status, body := doIdemBookingRequest(t, app, "/api/v1/bookings/1/payment/retry", "", nil)

	require.Equal(t, 200, status)
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok, "response must carry the booking: %v", body)
	require.NotNil(t, data["payment"], "the retry must return the payment")
	assert.Equal(t, "https://checkout.example/pay/1", data["payment"].(map[string]interface{})["payment_url"])
	assert.Equal(t, []string{key}, gateway.requestIDs, "the checkout is sent with the stored key, not a new one")
	assert.Len(t, bookingRepo.bookings, 1, "the retry never creates a booking")
}

func timePtr(t time.Time) *time.Time {
	return &t
}
