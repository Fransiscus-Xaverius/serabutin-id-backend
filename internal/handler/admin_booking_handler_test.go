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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 5: admin bookings ops — queue list with filters plus dispute
// force-cancel with mandatory reason.

type mockAdminBookingRepo struct {
	mock.Mock
}

func (m *mockAdminBookingRepo) Create(ctx context.Context, b *entity.Booking) error {
	return m.Called(ctx, b).Error(0)
}
func (m *mockAdminBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Booking), args.Error(1)
}
func (m *mockAdminBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return m.Called(ctx, id, status).Error(0)
}
func (m *mockAdminBookingRepo) UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error {
	return m.Called(ctx, id, scheduleDate, scheduleTime, address).Error(0)
}
func (m *mockAdminBookingRepo) UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error {
	return m.Called(ctx, id, cancellationID, cancelledBy).Error(0)
}
func (m *mockAdminBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	args := m.Called(ctx, userID, role, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Booking), args.Get(1).(int64), args.Error(2)
}
func (m *mockAdminBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	args := m.Called(ctx, providerID, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}
func (m *mockAdminBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Booking), args.Get(1).(int64), args.Error(2)
}

type mockAdminBookingCancelRepo struct {
	mock.Mock
}

func (m *mockAdminBookingCancelRepo) Create(ctx context.Context, c *entity.Cancellation) error {
	c.ID = 9
	return m.Called(ctx, c).Error(0)
}
func (m *mockAdminBookingCancelRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error) {
	args := m.Called(ctx, bookingID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Cancellation), args.Error(1)
}
func (m *mockAdminBookingCancelRepo) List(ctx context.Context, filter repository.CancellationFilter) ([]*entity.Cancellation, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Cancellation), args.Get(1).(int64), args.Error(2)
}
func (m *mockAdminBookingCancelRepo) CreateAndApplyCancel(ctx context.Context, c *entity.Cancellation, cancelledBy uint) error {
	c.ID = 9
	return m.Called(ctx, c, cancelledBy).Error(0)
}

func newAdminBookingTestApp(t *testing.T, bookingRepo repository.BookingRepository, cancelRepo repository.CancellationRepository) *fiber.App {
	t.Helper()
	uc := usecase.NewAdminBookingUsecase(bookingRepo, cancelRepo)
	h := NewAdminHandler(cancelRepo, nil, bookingRepo, nil, nil, nil, nil, nil, nil, uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/bookings", h.ListBookings)
	app.Patch("/api/v1/admin/bookings/:id/cancel", h.CancelBooking)
	return app
}

func doAdminBookingRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
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

func TestAdminHandler_ListBookings_ReturnsRowsAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	filter := repository.AdminBookingFilter{Page: 1, Limit: 20}
	bookingRepo.On("ListForAdmin", ctx, filter).Return([]*entity.Booking{
		{ID: 42, CustomerID: 10, ProviderID: 20, ServiceID: 11, Status: "pending", Total: 80000},
	}, int64(1), nil)
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	status, body := doAdminBookingRequest(t, app, "GET", "/api/v1/admin/bookings", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	row := rows[0].(map[string]interface{})
	assert.Equal(t, float64(42), row["id"])
	assert.Equal(t, "pending", row["status"])
	assert.Equal(t, float64(80000), row["total"])
	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(1), pagination["total"])
	bookingRepo.AssertExpectations(t)
}

func TestAdminHandler_ListBookings_ForwardsFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	filter := repository.AdminBookingFilter{
		Status: "pending", CustomerID: 10, ProviderID: 20,
		DateFrom: "2026-10-01", DateTo: "2026-10-31", Page: 2, Limit: 10,
	}
	bookingRepo.On("ListForAdmin", ctx, filter).Return([]*entity.Booking{}, int64(0), nil)
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	status, _ := doAdminBookingRequest(t, app, "GET", "/api/v1/admin/bookings?status=pending&customer_id=10&provider_id=20&date_from=2026-10-01&date_to=2026-10-31&page=2&limit=10", "")

	assert.Equal(t, fiber.StatusOK, status)
	bookingRepo.AssertExpectations(t)
}

func TestAdminHandler_CancelBooking_MissingReason_400(t *testing.T) {
	t.Parallel()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	for _, body := range []string{"", "{}", `{"reason":""}`, `{"reason":"   "}`} {
		status, resp := doAdminBookingRequest(t, app, "PATCH", "/api/v1/admin/bookings/42/cancel", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	bookingRepo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_CancelBooking_Pending_200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	bookingRepo.On("FindByID", ctx, uint(42)).Return(&entity.Booking{
		ID: 42, CustomerID: 10, ProviderID: 20, Status: "pending", SubTotal: 75000, PlatformFee: 5000,
	}, nil)
	cancelRepo.On("CreateAndApplyCancel", ctx, mock.AnythingOfType("*entity.Cancellation"), uint(7)).Return(nil)
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	status, body := doAdminBookingRequest(t, app, "PATCH", "/api/v1/admin/bookings/42/cancel", `{"reason":"penipuan terkonfirmasi"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(42), data["id"])
	assert.Equal(t, "cancelled", data["status"])
	bookingRepo.AssertExpectations(t)
	cancelRepo.AssertExpectations(t)
}

func TestAdminHandler_CancelBooking_Completed_422(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	bookingRepo.On("FindByID", ctx, uint(42)).Return(&entity.Booking{
		ID: 42, CustomerID: 10, ProviderID: 20, Status: "completed",
	}, nil)
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	status, body := doAdminBookingRequest(t, app, "PATCH", "/api/v1/admin/bookings/42/cancel", `{"reason":"sengketa"}`)

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", body["code"])
	bookingRepo.AssertNotCalled(t, "UpdateCancel")
}

func TestAdminHandler_CancelBooking_NotFound_404(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bookingRepo := new(mockAdminBookingRepo)
	cancelRepo := new(mockAdminBookingCancelRepo)
	bookingRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found"))
	app := newAdminBookingTestApp(t, bookingRepo, cancelRepo)

	status, body := doAdminBookingRequest(t, app, "PATCH", "/api/v1/admin/bookings/99/cancel", `{"reason":"sengketa"}`)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "BOOKING_NOT_FOUND", body["code"])
}
