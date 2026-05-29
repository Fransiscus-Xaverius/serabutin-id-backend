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
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type mockTokenRepo struct {
	mock.Mock
}

func (m *mockTokenRepo) StoreRefreshToken(ctx context.Context, userID uint, tokenUUID string, expires time.Duration) error {
	args := m.Called(ctx, userID, tokenUUID, expires)
	return args.Error(0)
}

func (m *mockTokenRepo) IsRefreshTokenValid(ctx context.Context, tokenUUID string) (uint, error) {
	args := m.Called(ctx, tokenUUID)
	return args.Get(0).(uint), args.Error(1)
}

func (m *mockTokenRepo) RevokeRefreshToken(ctx context.Context, tokenUUID string) error {
	args := m.Called(ctx, tokenUUID)
	return args.Error(0)
}

func (m *mockTokenRepo) RevokeAllUserTokens(ctx context.Context, userID uint) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *mockTokenRepo) BlacklistAccessToken(ctx context.Context, tokenUUID string, expires time.Duration) error {
	args := m.Called(ctx, tokenUUID, expires)
	return args.Error(0)
}

func (m *mockTokenRepo) IsAccessTokenBlacklisted(ctx context.Context, tokenUUID string) (bool, error) {
	args := m.Called(ctx, tokenUUID)
	return args.Bool(0), args.Error(1)
}

func (m *mockTokenRepo) StoreTokenPair(ctx context.Context, td *entity.TokenDetails, userID uint) error {
	args := m.Called(ctx, td, userID)
	return args.Error(0)
}

func generateTestToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

type apiErrorResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	RequestID string      `json:"request_id"`
	Code      string      `json:"code"`
	Details   interface{} `json:"details,omitempty"`
}

func setupAuthTestApp(t *testing.T, mw *AuthMiddleware) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: apierror.GlobalErrorHandler,
	})

	app.Use(mw.Authenticate())
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	return app
}

func TestAuthMiddleware_Authenticate_ValidToken_SetsUserContext(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mRepo := new(mockTokenRepo)
	mw := NewAuthMiddleware(secret, mRepo)

	claims := jwt.MapClaims{
		"user_id":     float64(42),
		"email":       "user@example.com",
		"access_uuid": "test-access-uuid",
		"exp":         time.Now().Add(15 * time.Minute).Unix(),
		"iat":         time.Now().Unix(),
	}
	token := generateTestToken(t, secret, claims)

	mRepo.On("IsAccessTokenBlacklisted", mock.Anything, "test-access-uuid").Return(false, nil)

	app := setupAuthTestApp(t, mw)

	app.Get("/check-context", func(c fiber.Ctx) error {
		userID := requestctx.GetUserID(c)
		email := requestctx.GetUserEmail(c)
		return c.JSON(fiber.Map{
			"user_id": userID,
			"email":   email,
		})
	})

	req := httptest.NewRequest("GET", "/check-context", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result fiber.Map
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, float64(42), result["user_id"])
	assert.Equal(t, "user@example.com", result["email"])

	mRepo.AssertExpectations(t)
}

func TestAuthMiddleware_Authenticate_BlacklistedToken_Returns401(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mRepo := new(mockTokenRepo)
	mw := NewAuthMiddleware(secret, mRepo)

	claims := jwt.MapClaims{
		"user_id":     float64(1),
		"email":       "user@example.com",
		"access_uuid": "blacklisted-uuid",
		"exp":         time.Now().Add(15 * time.Minute).Unix(),
		"iat":         time.Now().Unix(),
	}
	token := generateTestToken(t, secret, claims)

	mRepo.On("IsAccessTokenBlacklisted", mock.Anything, "blacklisted-uuid").Return(true, nil)

	app := setupAuthTestApp(t, mw)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "TOKEN_REVOKED", apiResp.Code)

	mRepo.AssertExpectations(t)
}

func TestAuthMiddleware_Authenticate_NoHeader_Returns401(t *testing.T) {
	t.Parallel()

	mw := NewAuthMiddleware("test-secret", nil)
	app := setupAuthTestApp(t, mw)

	req := httptest.NewRequest("GET", "/protected", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "MISSING_TOKEN", apiResp.Code)
}

func TestAuthMiddleware_Authenticate_MalformedHeader_Returns401(t *testing.T) {
	t.Parallel()

	mw := NewAuthMiddleware("test-secret", nil)
	app := setupAuthTestApp(t, mw)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "InvalidFormat")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "INVALID_HEADER", apiResp.Code)
}

func TestAuthMiddleware_Authenticate_ExpiredToken_Returns401(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	mw := NewAuthMiddleware(secret, nil)

	claims := jwt.MapClaims{
		"user_id":     float64(1),
		"email":       "user@example.com",
		"access_uuid": "expired-uuid",
		"exp":         time.Now().Add(-1 * time.Hour).Unix(),
		"iat":         time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := generateTestToken(t, secret, claims)

	app := setupAuthTestApp(t, mw)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "INVALID_TOKEN", apiResp.Code)
}
