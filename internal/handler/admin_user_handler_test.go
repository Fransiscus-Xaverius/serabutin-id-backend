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
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 3: admin user endpoints — list/get/mutations, mandatory reason.

func newAdminUserTestApp(t *testing.T, repo repository.UserRepository) *fiber.App {
	t.Helper()
	uc := usecase.NewAdminUserUsecase(repo)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, uc, nil, nil)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/users", h.ListUsers)
	app.Get("/api/v1/admin/users/:id", h.GetUser)
	app.Patch("/api/v1/admin/users/:id/suspend", h.SuspendUser)
	app.Patch("/api/v1/admin/users/:id/restore", h.RestoreUser)
	app.Patch("/api/v1/admin/users/:id/role", h.SetUserRole)
	return app
}

func doAdminUserRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
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

func TestAdminHandler_ListUsers_ReturnsRowsAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	filter := repository.UserFilter{Page: 1, Limit: 20}
	repo.On("List", ctx, filter).Return([]*entity.User{
		{ID: 3, Email: "budi@example.com", Name: "Budi", WhatsApp: "6281", Role: "customer", Status: "active"},
	}, int64(1), nil)
	app := newAdminUserTestApp(t, repo)

	status, body := doAdminUserRequest(t, app, "GET", "/api/v1/admin/users", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	assert.Equal(t, "budi@example.com", rows[0].(map[string]interface{})["email"])
	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(1), pagination["total"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_ListUsers_ForwardsSearchAndFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	filter := repository.UserFilter{Q: "budi", Role: "provider", Status: "suspended", Page: 2, Limit: 10}
	repo.On("List", ctx, filter).Return([]*entity.User{}, int64(0), nil)
	app := newAdminUserTestApp(t, repo)

	status, _ := doAdminUserRequest(t, app, "GET", "/api/v1/admin/users?q=budi&role=provider&status=suspended&page=2&limit=10", "")

	assert.Equal(t, fiber.StatusOK, status)
	repo.AssertExpectations(t)
}

func TestAdminHandler_GetUser_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))
	app := newAdminUserTestApp(t, repo)

	status, body := doAdminUserRequest(t, app, "GET", "/api/v1/admin/users/99", "")

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "USER_NOT_FOUND", body["code"])
}

func TestAdminHandler_SuspendUser_MissingReason_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminUserTestApp(t, repo)

	for _, body := range []string{"", "{}", `{"reason":""}`} {
		status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/suspend", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SuspendUser_WithReason_Suspends(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(3)).Return(&entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: "active"}, nil)
	repo.On("UpdateStatus", ctx, uint(3), entity.UserStatusSuspended).Return(nil)
	app := newAdminUserTestApp(t, repo)

	status, body := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/suspend", `{"reason":"spam reports"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "suspended", data["status"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_RestoreUser_MissingReason_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminUserTestApp(t, repo)

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/restore", "{}")

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_RestoreUser_WithReason_Restores(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(3)).Return(&entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: "suspended"}, nil)
	repo.On("UpdateStatus", ctx, uint(3), entity.UserStatusActive).Return(nil)
	app := newAdminUserTestApp(t, repo)

	status, body := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/restore", `{"reason":"appeal accepted"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "active", data["status"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_SetUserRole_MissingReason_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminUserTestApp(t, repo)

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/role", `{"role":"provider"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetUserRole_InvalidRole_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminUserTestApp(t, repo)

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/role", `{"role":"superadmin","reason":"x"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
}

func TestAdminHandler_SetUserRole_SelfDemote_403(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(7)).Return(&entity.User{ID: 7, Email: "admin@example.com", Role: "admin", Status: "active"}, nil)
	app := newAdminUserTestApp(t, repo)

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/7/role", `{"role":"customer","reason":"stepping down"}`)

	assert.Equal(t, fiber.StatusForbidden, status)
	assert.Equal(t, "SELF_DEMOTE", resp["code"])
	repo.AssertNotCalled(t, "UpdateRole")
}

func TestAdminHandler_SetUserRole_Promotes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(3)).Return(&entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: "active"}, nil)
	repo.On("UpdateRole", ctx, uint(3), "provider").Return(nil)
	app := newAdminUserTestApp(t, repo)

	status, body := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/3/role", `{"role":"provider","reason":"verified"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "provider", data["role"])
	repo.AssertExpectations(t)
}
