package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

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

// Ticket 4: admin jasa moderation endpoints — queue list/get plus
// approve/reject/takedown with mandatory reason.

type mockServiceRepo struct {
	mock.Mock
}

func (m *mockServiceRepo) Create(ctx context.Context, service *entity.Service) error {
	return m.Called(ctx, service).Error(0)
}

func (m *mockServiceRepo) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Service), args.Error(1)
}

func (m *mockServiceRepo) Update(ctx context.Context, service *entity.Service) error {
	return m.Called(ctx, service).Error(0)
}

func (m *mockServiceRepo) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func (m *mockServiceRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, providerID, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func (m *mockServiceRepo) SetStatus(ctx context.Context, id uint, status string) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *mockServiceRepo) ListForAdmin(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func newAdminServiceTestApp(t *testing.T, repo repository.ServiceRepository) *fiber.App {
	t.Helper()
	uc := usecase.NewAdminServiceUsecase(repo)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, uc, nil, nil)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/services", h.ListServices)
	app.Get("/api/v1/admin/services/:id", h.GetService)
	app.Patch("/api/v1/admin/services/:id/status", h.SetServiceStatus)
	return app
}

func doAdminServiceRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
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

func TestAdminHandler_ListServices_ReturnsRowsAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	filter := repository.ServiceFilter{Page: 1, Limit: 20}
	repo.On("ListForAdmin", ctx, filter).Return([]*entity.Service{
		{ID: 11, ProviderID: 4, Title: "Bersih Rumah", Category: "cleaning", BasePrice: 75000, Status: "draft"},
	}, int64(1), nil)
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "GET", "/api/v1/admin/services", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	assert.Equal(t, "Bersih Rumah", rows[0].(map[string]interface{})["title"])
	assert.Equal(t, "draft", rows[0].(map[string]interface{})["status"])
	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(1), pagination["total"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_ListServices_ForwardsFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	filter := repository.ServiceFilter{Query: "bersih", Category: "cleaning", Status: "draft,pending", Page: 2, Limit: 10}
	repo.On("ListForAdmin", ctx, filter).Return([]*entity.Service{}, int64(0), nil)
	app := newAdminServiceTestApp(t, repo)

	status, _ := doAdminServiceRequest(t, app, "GET", "/api/v1/admin/services?q=bersih&category=cleaning&status=draft,pending&page=2&limit=10", "")

	assert.Equal(t, fiber.StatusOK, status)
	repo.AssertExpectations(t)
}

func TestAdminHandler_GetService_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	repo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found"))
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "GET", "/api/v1/admin/services/99", "")

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "SERVICE_NOT_FOUND", body["code"])
}

func TestAdminHandler_SetServiceStatus_MissingReason_400(t *testing.T) {
	t.Parallel()
	repo := new(mockServiceRepo)
	app := newAdminServiceTestApp(t, repo)

	for _, body := range []string{"", "{}", `{"status":"active"}`, `{"status":"active","reason":""}`} {
		status, resp := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetServiceStatus_MissingStatus_400(t *testing.T) {
	t.Parallel()
	repo := new(mockServiceRepo)
	app := newAdminServiceTestApp(t, repo)

	status, resp := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", `{"reason":"looks good"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetServiceStatus_ApproveDraft_200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	repo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, ProviderID: 4, Title: "Bersih Rumah", Category: "cleaning", Status: "draft"}, nil)
	repo.On("SetStatus", ctx, uint(11), "active").Return(nil)
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", `{"status":"active","reason":"katalog valid"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "active", data["status"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_SetServiceStatus_InvalidTarget_400(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	repo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: "active"}, nil)
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", `{"status":"draft","reason":"oops"}`)

	// "draft" is not an admin target at all → 400 validation.
	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", body["code"])
	repo.AssertNotCalled(t, "SetStatus")
}

func TestAdminHandler_SetServiceStatus_DisallowedTransition_422(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	// taken_down -> rejected is not in the state machine.
	repo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: "taken_down"}, nil)
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", `{"status":"rejected","reason":"duplikat"}`)

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", body["code"])
	repo.AssertNotCalled(t, "SetStatus")
}

func TestAdminHandler_SetServiceStatus_TakedownActive_200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockServiceRepo)
	repo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: "active"}, nil)
	repo.On("SetStatus", ctx, uint(11), "taken_down").Return(nil)
	app := newAdminServiceTestApp(t, repo)

	status, body := doAdminServiceRequest(t, app, "PATCH", "/api/v1/admin/services/11/status", `{"status":"taken_down","reason":"katalog plagiat"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "taken_down", data["status"])
	repo.AssertExpectations(t)
}
