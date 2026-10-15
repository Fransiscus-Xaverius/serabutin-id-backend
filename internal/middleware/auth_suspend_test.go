package middleware

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// F1: Authenticate rejects suspended accounts (401 ACCOUNT_SUSPENDED) and
// RequireAdmin locks suspended admins out of /admin (403).

type mockStatusUserRepo struct {
	mock.Mock
}

func (m *mockStatusUserRepo) Create(ctx context.Context, user *entity.User) error {
	return m.Called(ctx, user).Error(0)
}
func (m *mockStatusUserRepo) Update(ctx context.Context, user *entity.User) error {
	return m.Called(ctx, user).Error(0)
}
func (m *mockStatusUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockStatusUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockStatusUserRepo) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	args := m.Called(ctx, whatsapp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockStatusUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}
func (m *mockStatusUserRepo) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.User), args.Get(1).(int64), args.Error(2)
}
func (m *mockStatusUserRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return m.Called(ctx, id, status).Error(0)
}
func (m *mockStatusUserRepo) UpdateRole(ctx context.Context, id uint, role string) error {
	return m.Called(ctx, id, role).Error(0)
}
func (m *mockStatusUserRepo) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	return m.Called(ctx, id, blocked).Error(0)
}
func (m *mockStatusUserRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func suspendedTestToken(t *testing.T, secret string, userID uint, uuid string) string {
	t.Helper()
	return generateTestToken(t, secret, jwt.MapClaims{
		"user_id":     float64(userID),
		"email":       "user@example.com",
		"access_uuid": uuid,
		"exp":         time.Now().Add(15 * time.Minute).Unix(),
		"iat":         time.Now().Unix(),
	})
}

func TestAuthMiddleware_Authenticate_SuspendedUser_Returns401(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mRepo := new(mockTokenRepo)
	uRepo := new(mockStatusUserRepo)
	mw := NewAuthMiddleware(secret, mRepo)
	mw.SetUserRepository(uRepo)

	token := suspendedTestToken(t, secret, 9, "uuid-suspended")
	mRepo.On("IsAccessTokenBlacklisted", mock.Anything, "uuid-suspended").Return(false, nil)
	uRepo.On("FindByID", mock.Anything, uint(9)).Return(&entity.User{
		ID: 9, Role: "customer", Status: entity.UserStatusSuspended,
	}, nil)

	app := setupAuthTestApp(t, mw)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "ACCOUNT_SUSPENDED", apiResp.Code)
	mRepo.AssertExpectations(t)
	uRepo.AssertExpectations(t)
}

func TestAuthMiddleware_Authenticate_ActiveUser_Passes(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mRepo := new(mockTokenRepo)
	uRepo := new(mockStatusUserRepo)
	mw := NewAuthMiddleware(secret, mRepo)
	mw.SetUserRepository(uRepo)

	token := suspendedTestToken(t, secret, 9, "uuid-active")
	mRepo.On("IsAccessTokenBlacklisted", mock.Anything, "uuid-active").Return(false, nil)
	uRepo.On("FindByID", mock.Anything, uint(9)).Return(&entity.User{
		ID: 9, Role: "customer", Status: entity.UserStatusActive,
	}, nil)

	app := setupAuthTestApp(t, mw)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestAuthMiddleware_Authenticate_StatusLookupError_FailsOpen(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mRepo := new(mockTokenRepo)
	uRepo := new(mockStatusUserRepo)
	mw := NewAuthMiddleware(secret, mRepo)
	mw.SetUserRepository(uRepo)

	token := suspendedTestToken(t, secret, 9, "uuid-flaky")
	mRepo.On("IsAccessTokenBlacklisted", mock.Anything, "uuid-flaky").Return(false, nil)
	uRepo.On("FindByID", mock.Anything, uint(9)).Return(nil, assert.AnError)

	app := setupAuthTestApp(t, mw)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

type stubProfileLoader struct {
	user *entity.User
	err  error
}

func (s *stubProfileLoader) GetProfile(ctx context.Context, userID uint) (*entity.User, error) {
	return s.user, s.err
}

func TestRequireAdmin_SuspendedAdmin_Forbidden(t *testing.T) {
	t.Parallel()

	newAdminApp := func(loader profileLoader) *fiber.App {
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			requestctx.SetUserID(c, 7)
			return c.Next()
		})
		app.Use(RequireAdmin(loader))
		app.Get("/admin/ping", func(c fiber.Ctx) error {
			return c.SendString("OK")
		})
		return app
	}

	suspended := newAdminApp(&stubProfileLoader{user: &entity.User{
		ID: 7, Role: "admin", Status: entity.UserStatusSuspended,
	}})
	req := httptest.NewRequest("GET", "/admin/ping", nil)
	resp, err := suspended.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "ACCOUNT_SUSPENDED", apiResp.Code)

	active := newAdminApp(&stubProfileLoader{user: &entity.User{
		ID: 7, Role: "admin", Status: entity.UserStatusActive,
	}})
	req2 := httptest.NewRequest("GET", "/admin/ping", nil)
	resp2, err := active.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)
}
