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

// Ticket 11: finance overview + reimbursement queue — overview shape,
// transactions list, payouts with manual paid mark, decide requires reason
// (400), re-decide is 422, filing as non-owner is 403.

type mockFinanceRepo struct {
	mock.Mock
}

func (m *mockFinanceRepo) Summary(ctx context.Context, window repository.FinanceWindow) (repository.FinanceSummary, error) {
	args := m.Called(ctx, window)
	return args.Get(0).(repository.FinanceSummary), args.Error(1)
}

func (m *mockFinanceRepo) Transactions(ctx context.Context, filter repository.TransactionFilter) ([]repository.FinanceTransaction, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]repository.FinanceTransaction), args.Get(1).(int64), args.Error(2)
}

func (m *mockFinanceRepo) Payouts(ctx context.Context, period string) ([]repository.FinancePayout, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.FinancePayout), args.Error(1)
}

func (m *mockFinanceRepo) MarkPayoutPaid(ctx context.Context, mark *entity.PayoutMark) error {
	return m.Called(ctx, mark).Error(0)
}

type mockReimbRepo struct {
	mock.Mock
}

func (m *mockReimbRepo) Create(ctx context.Context, r *entity.Reimbursement) error {
	return m.Called(ctx, r).Error(0)
}

func (m *mockReimbRepo) FindByID(ctx context.Context, id uint) (*entity.Reimbursement, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Reimbursement), args.Error(1)
}

func (m *mockReimbRepo) List(ctx context.Context, filter repository.ReimbursementFilter) ([]*entity.Reimbursement, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Reimbursement), args.Get(1).(int64), args.Error(2)
}

func (m *mockReimbRepo) Decide(ctx context.Context, id uint, decidedBy uint, status string) error {
	return m.Called(ctx, id, decidedBy, status).Error(0)
}

type mockFinanceBookingRepo struct {
	mock.Mock
}

func (m *mockFinanceBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Booking), args.Error(1)
}

func newAdminFinanceTestApp(t *testing.T, finance repository.FinanceRepository, reimb repository.ReimbursementRepository, bookings usecase.FinanceBookingReader, actor uint) (*fiber.App, *ReimbursementHandler) {
	t.Helper()
	uc := usecase.NewAdminFinanceUsecase(finance, reimb, bookings)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminFinanceUC(uc)
	rh := NewReimbursementHandler(uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, actor)
		return c.Next()
	})
	app.Get("/api/v1/admin/finance/overview", h.GetFinanceOverview)
	app.Get("/api/v1/admin/finance/summary", h.GetFinanceSummary)
	app.Get("/api/v1/admin/transactions", h.ListTransactions)
	app.Get("/api/v1/admin/payouts", h.ListPayouts)
	app.Post("/api/v1/admin/payouts/mark", h.MarkPayoutPaid)
	app.Get("/api/v1/admin/reimbursements", h.ListReimbursements)
	app.Patch("/api/v1/admin/reimbursements/:id", h.DecideReimbursement)
	app.Post("/api/v1/reimbursements", rh.File)
	return app, rh
}

func doAdminFinanceRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
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

func financeTestSummary() repository.FinanceSummary {
	return repository.FinanceSummary{
		GMV:             2400000,
		PlatformFees:    160000,
		ByPaymentMethod: map[string]int64{"qris": 2400000},
		CompletedCount:  20,
		CancelledCount:  3,
	}
}

func TestAdminHandler_GetFinanceOverview_ReturnsCombinedView(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	bookings := new(mockFinanceBookingRepo)
	now := time.Now().UTC()
	finance.On("Summary", mock.Anything, mock.Anything).Return(financeTestSummary(), nil)
	finance.On("Transactions", mock.Anything, mock.Anything).Return([]repository.FinanceTransaction{
		{ID: 42, CustomerID: 10, ProviderID: 20, ServiceID: 11, SubTotal: 470000, PlatformFee: 30000, Total: 500000, PaymentMethod: "qris", Status: "completed", CreatedAt: now},
	}, int64(1), nil)
	finance.On("Payouts", mock.Anything, mock.Anything).Return([]repository.FinancePayout{
		{ProviderID: 20, Period: "2026-10", Gross: 500000, Fee: 30000, Net: 470000, Status: entity.PayoutPending},
	}, nil)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, bookings, 1)

	status, body := doAdminFinanceRequest(t, app, "GET", "/api/v1/admin/finance/overview", "")
	require.Equal(t, 200, status)
	data := body["data"].(map[string]interface{})
	summary := data["summary"].(map[string]interface{})
	assert.Equal(t, float64(2400000), summary["gmv"])
	assert.Equal(t, float64(160000), summary["platform_fees"])
	assert.Len(t, data["transactions"].([]interface{}), 1)
	assert.Len(t, data["payouts"].([]interface{}), 1)
}

func TestAdminHandler_GetFinanceOverview_BadDateRange400(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	app, _ := newAdminFinanceTestApp(t, finance, new(mockReimbRepo), new(mockFinanceBookingRepo), 1)

	status, _ := doAdminFinanceRequest(t, app, "GET", "/api/v1/admin/finance/overview?from=not-a-date", "")
	assert.Equal(t, 400, status)
	finance.AssertNotCalled(t, "Summary", mock.Anything, mock.Anything)
}

func TestAdminHandler_ListReimbursements_ReturnsQueue(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	reimb.On("List", mock.Anything, repository.ReimbursementFilter{Page: 1, Limit: 20}).Return([]*entity.Reimbursement{
		{ID: 5, BookingID: 42, FiledBy: 20, Amount: 50000, Reason: "biaya tol", Status: entity.ReimbursementPending, CreatedAt: time.Now().UTC()},
	}, int64(1), nil)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, new(mockFinanceBookingRepo), 1)

	status, body := doAdminFinanceRequest(t, app, "GET", "/api/v1/admin/reimbursements", "")
	require.Equal(t, 200, status)
	data := body["data"].(map[string]interface{})
	assert.Len(t, data["data"].([]interface{}), 1)
}

func TestAdminHandler_DecideReimbursement_RequiresReason400(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, new(mockFinanceBookingRepo), 1)

	status, _ := doAdminFinanceRequest(t, app, "PATCH", "/api/v1/admin/reimbursements/5", `{"decision":"approved"}`)
	assert.Equal(t, 400, status)
	reimb.AssertNotCalled(t, "Decide", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAdminHandler_DecideReimbursement_Approve200(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	reimb.On("FindByID", mock.Anything, uint(5)).Return(&entity.Reimbursement{
		ID: 5, BookingID: 42, FiledBy: 20, Amount: 50000, Reason: "biaya tol",
		Status: entity.ReimbursementPending, CreatedAt: time.Now().UTC(),
	}, nil)
	reimb.On("Decide", mock.Anything, uint(5), uint(1), "approved").Return(nil)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, new(mockFinanceBookingRepo), 1)

	status, body := doAdminFinanceRequest(t, app, "PATCH", "/api/v1/admin/reimbursements/5", `{"decision":"approved","reason":"bukti valid"}`)
	require.Equal(t, 200, status)
	assert.Equal(t, "approved", body["data"].(map[string]interface{})["status"])
}

func TestAdminHandler_DecideReimbursement_DoubleDecide422(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	reimb.On("FindByID", mock.Anything, uint(5)).Return(&entity.Reimbursement{
		ID: 5, BookingID: 42, FiledBy: 20, Amount: 50000, Reason: "biaya tol",
		Status: entity.ReimbursementApproved, CreatedAt: time.Now().UTC(),
	}, nil)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, new(mockFinanceBookingRepo), 1)

	status, _ := doAdminFinanceRequest(t, app, "PATCH", "/api/v1/admin/reimbursements/5", `{"decision":"rejected","reason":"duplikat"}`)
	assert.Equal(t, 422, status)
}

func TestAdminHandler_FileReimbursement_NonOwner403(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	reimb := new(mockReimbRepo)
	bookings := new(mockFinanceBookingRepo)
	bookings.On("FindByID", mock.Anything, uint(42)).Return(&entity.Booking{
		ID: 42, CustomerID: 10, ProviderID: 20, Status: "completed",
	}, nil)
	app, _ := newAdminFinanceTestApp(t, finance, reimb, bookings, 10)

	status, _ := doAdminFinanceRequest(t, app, "POST", "/api/v1/reimbursements", `{"booking_id":42,"amount":50000,"reason":"biaya tol"}`)
	assert.Equal(t, 403, status)
}

func TestAdminHandler_FileReimbursement_MissingAmount400(t *testing.T) {
	t.Parallel()
	app, _ := newAdminFinanceTestApp(t, new(mockFinanceRepo), new(mockReimbRepo), new(mockFinanceBookingRepo), 20)

	status, _ := doAdminFinanceRequest(t, app, "POST", "/api/v1/reimbursements", `{"booking_id":42,"reason":"biaya tol"}`)
	assert.Equal(t, 400, status)
}

func TestAdminHandler_MarkPayoutPaid_RequiresReason400(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	app, _ := newAdminFinanceTestApp(t, finance, new(mockReimbRepo), new(mockFinanceBookingRepo), 1)

	status, _ := doAdminFinanceRequest(t, app, "POST", "/api/v1/admin/payouts/mark", `{"provider_id":20,"period":"2026-10"}`)
	assert.Equal(t, 400, status)
	finance.AssertNotCalled(t, "MarkPayoutPaid", mock.Anything, mock.Anything)
}

func TestAdminHandler_MarkPayoutPaid_BadPeriod400(t *testing.T) {
	t.Parallel()
	finance := new(mockFinanceRepo)
	app, _ := newAdminFinanceTestApp(t, finance, new(mockReimbRepo), new(mockFinanceBookingRepo), 1)

	status, _ := doAdminFinanceRequest(t, app, "POST", "/api/v1/admin/payouts/mark", `{"provider_id":20,"period":"october","reason":"transfer manual selesai"}`)
	assert.Equal(t, 400, status)
}

func TestAdminHandler_FileReimbursement_UnknownBooking404(t *testing.T) {
	t.Parallel()
	bookings := new(mockFinanceBookingRepo)
	bookings.On("FindByID", mock.Anything, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found"))
	app, _ := newAdminFinanceTestApp(t, new(mockFinanceRepo), new(mockReimbRepo), bookings, 20)

	status, _ := doAdminFinanceRequest(t, app, "POST", "/api/v1/reimbursements", `{"booking_id":99,"amount":50000,"reason":"biaya tol"}`)
	assert.Equal(t, 404, status)
}
