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

// Ticket 6: admin reviews/reports moderation — flagged queue list, hide/keep
// with mandatory reason, report queue list, resolve/dismiss with mandatory
// reason, 422 on re-resolve.

type mockReviewRepo struct {
	mock.Mock
}

func (m *mockReviewRepo) Create(ctx context.Context, review *entity.Review) error {
	return m.Called(ctx, review).Error(0)
}

func (m *mockReviewRepo) FindByID(ctx context.Context, id uint) (*entity.Review, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Review), args.Error(1)
}

func (m *mockReviewRepo) FindByServiceID(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, error) {
	args := m.Called(ctx, serviceID, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Review), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepo) FindByProviderID(ctx context.Context, providerID uint, page, limit int) ([]*entity.Review, int64, error) {
	args := m.Called(ctx, providerID, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Review), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepo) FindReceivedByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Review, int64, error) {
	args := m.Called(ctx, userID, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Review), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepo) GetServiceStats(ctx context.Context, serviceID uint) (*repository.ServiceReviewStats, error) {
	args := m.Called(ctx, serviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.ServiceReviewStats), args.Error(1)
}

func (m *mockReviewRepo) ExistsByBookingAndAuthor(ctx context.Context, bookingID, authorID uint) (bool, error) {
	args := m.Called(ctx, bookingID, authorID)
	return args.Bool(0), args.Error(1)
}

func (m *mockReviewRepo) ListForAdmin(ctx context.Context, filter repository.AdminReviewFilter) ([]*entity.Review, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Review), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepo) SetHidden(ctx context.Context, id uint, hidden bool) error {
	return m.Called(ctx, id, hidden).Error(0)
}

type mockReportRepo struct {
	mock.Mock
}

func (m *mockReportRepo) Create(ctx context.Context, report *entity.Report) error {
	return m.Called(ctx, report).Error(0)
}

func (m *mockReportRepo) FindByID(ctx context.Context, id uint) (*entity.Report, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Report), args.Error(1)
}

func (m *mockReportRepo) ListForAdmin(ctx context.Context, filter repository.ReportFilter) ([]*entity.Report, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Report), args.Get(1).(int64), args.Error(2)
}

func (m *mockReportRepo) Resolve(ctx context.Context, id uint, adminID uint, status string) error {
	return m.Called(ctx, id, adminID, status).Error(0)
}

func newAdminReviewTestApp(t *testing.T, reviews repository.ReviewRepository, reports repository.ReportRepository) *fiber.App {
	t.Helper()
	uc := usecase.NewAdminReviewUsecase(reviews, reports)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminReviewUC(uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/reviews", h.ListReviews)
	app.Patch("/api/v1/admin/reviews/:id/visibility", h.SetReviewVisibility)
	app.Get("/api/v1/admin/reports", h.ListReports)
	app.Patch("/api/v1/admin/reports/:id", h.ResolveReport)
	return app
}

func doAdminReviewRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
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

func TestAdminHandler_ListReviews_ReturnsRowsWithHiddenFlag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	filter := repository.AdminReviewFilter{Page: 1, Limit: 20}
	reviews.On("ListForAdmin", ctx, filter).Return([]*entity.Review{
		{ID: 7, BookingID: 42, ServiceID: 11, CustomerID: 10, ProviderID: 20, AuthorID: 10, AuthorRole: "customer", Rating: 1, Comment: "bahasa kasar", Hidden: true, CreatedAt: time.Now()},
	}, int64(1), nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "GET", "/api/v1/admin/reviews", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	assert.Equal(t, true, rows[0].(map[string]interface{})["hidden"])
	assert.Equal(t, "bahasa kasar", rows[0].(map[string]interface{})["comment"])
	reviews.AssertExpectations(t)
}

func TestAdminHandler_ListReviews_FlaggedNarrowsToHidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	filter := repository.AdminReviewFilter{Page: 1, Limit: 20, HiddenOnly: true}
	reviews.On("ListForAdmin", ctx, filter).Return([]*entity.Review{}, int64(0), nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, _ := doAdminReviewRequest(t, app, "GET", "/api/v1/admin/reviews?flagged=1", "")

	assert.Equal(t, fiber.StatusOK, status)
	reviews.AssertExpectations(t)
}

func TestAdminHandler_SetReviewVisibility_MissingReason_400(t *testing.T) {
	t.Parallel()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	app := newAdminReviewTestApp(t, reviews, reports)

	for _, body := range []string{"", "{}", `{"hidden":true}`, `{"hidden":true,"reason":""}`} {
		status, resp := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reviews/7/visibility", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	reviews.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetReviewVisibility_Hide_200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	reviews.On("FindByID", ctx, uint(7)).Return(&entity.Review{ID: 7, Rating: 1, Comment: "x"}, nil)
	reviews.On("SetHidden", ctx, uint(7), true).Return(nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reviews/7/visibility", `{"hidden":true,"reason":"bahasa kasar"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, true, data["hidden"])
	reviews.AssertExpectations(t)
}

func TestAdminHandler_SetReviewVisibility_NotFound_404(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	reviews.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "REVIEW_NOT_FOUND", "review not found"))
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reviews/99/visibility", `{"hidden":true,"reason":"spam"}`)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "REVIEW_NOT_FOUND", body["code"])
}

func TestAdminHandler_ListReports_ForwardsFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	filter := repository.ReportFilter{Status: "open", TargetType: "review", Page: 1, Limit: 20}
	reports.On("ListForAdmin", ctx, filter).Return([]*entity.Report{
		{ID: 3, ReporterID: 10, TargetType: "review", TargetID: 7, Reason: "bahasa kasar", Status: "open", CreatedAt: time.Now()},
	}, int64(1), nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "GET", "/api/v1/admin/reports?status=open&target_type=review", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	assert.Equal(t, "open", rows[0].(map[string]interface{})["status"])
	assert.Equal(t, "review", rows[0].(map[string]interface{})["target_type"])
	reports.AssertExpectations(t)
}

func TestAdminHandler_ResolveReport_MissingReason_400(t *testing.T) {
	t.Parallel()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	app := newAdminReviewTestApp(t, reviews, reports)

	for _, body := range []string{"", "{}", `{"status":"resolved"}`, `{"status":"resolved","reason":""}`} {
		status, resp := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reports/3", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	reports.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_ResolveReport_InvalidStatus_400(t *testing.T) {
	t.Parallel()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, resp := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reports/3", `{"status":"archived","reason":"x"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
	reports.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_ResolveReport_Resolved_200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	reports.On("FindByID", ctx, uint(3)).Return(&entity.Report{ID: 3, Status: "open"}, nil)
	reports.On("Resolve", ctx, uint(3), uint(7), "resolved").Return(nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reports/3", `{"status":"resolved","reason":"review disembunyikan"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "resolved", data["status"])
	reports.AssertExpectations(t)
}

func TestAdminHandler_ResolveReport_AlreadyResolved_422(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	reports.On("FindByID", ctx, uint(3)).Return(&entity.Report{ID: 3, Status: "resolved"}, nil)
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reports/3", `{"status":"resolved","reason":"x"}`)

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", body["code"])
	reports.AssertNotCalled(t, "Resolve")
}

func TestAdminHandler_ResolveReport_NotFound_404(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reviews := new(mockReviewRepo)
	reports := new(mockReportRepo)
	reports.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "REPORT_NOT_FOUND", "report not found"))
	app := newAdminReviewTestApp(t, reviews, reports)

	status, body := doAdminReviewRequest(t, app, "PATCH", "/api/v1/admin/reports/99", `{"status":"dismissed","reason":"x"}`)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "REPORT_NOT_FOUND", body["code"])
}
