package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

type mockUserRepo struct {
	mock.Mock
}

func (m *mockUserRepo) Create(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}

func (m *mockUserRepo) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	args := m.Called(ctx, whatsapp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserRepo) Update(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepo) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.User), args.Get(1).(int64), args.Error(2)
}

func (m *mockUserRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateRole(ctx context.Context, id uint, role string) error {
	args := m.Called(ctx, id, role)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	args := m.Called(ctx, id, blocked)
	return args.Error(0)
}

func (m *mockUserRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

type mockLoginLimiter struct {
	mock.Mock
}

func (m *mockLoginLimiter) IsLocked(ctx context.Context, identifier string) (bool, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Error(1)
}

func (m *mockLoginLimiter) RecordFailure(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

func (m *mockLoginLimiter) Reset(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

func (m *mockLoginLimiter) RemainingAttempts(ctx context.Context, identifier string) (int, error) {
	args := m.Called(ctx, identifier)
	return args.Int(0), args.Error(1)
}

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

type apiErrorResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	RequestID string      `json:"request_id"`
	Code      string      `json:"code"`
	Details   interface{} `json:"details,omitempty"`
}

func setupTestApp(t *testing.T) (*fiber.App, *mockUserRepo, *mockTokenRepo, *mockLoginLimiter, *service.TokenService) {
	t.Helper()

	mUserRepo := new(mockUserRepo)
	mTokenRepo := new(mockTokenRepo)
	mLimiter := new(mockLoginLimiter)
	tokenSvc := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)

	uc := usecase.NewAuthUsecase(mUserRepo, mTokenRepo, tokenSvc, mLimiter, bcrypt.MinCost)
	handler := NewAuthHandler(uc)

	app := fiber.New(fiber.Config{
		ErrorHandler: apierror.GlobalErrorHandler,
	})

	app.Post("/register", handler.Register)
	app.Post("/login", handler.Login)
	app.Post("/logout", handler.Logout)
	app.Post("/refresh", handler.RefreshToken)

	return app, mUserRepo, mTokenRepo, mLimiter, tokenSvc
}

func TestAuthHandler_Register_ValidRequest_Returns201(t *testing.T) {
	t.Parallel()

	app, mUserRepo, mTokenRepo, mLimiter, _ := setupTestApp(t)

	mUserRepo.On("ExistsByEmail", mock.Anything, "test@example.com").Return(false, nil)
	mUserRepo.On("FindByWhatsApp", mock.Anything, "+6281234567890").Return((*entity.User)(nil), apperror.ErrNotFound)
	mUserRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *entity.User) bool {
		return u.Email == "test@example.com" && u.Name == "Test User"
	})).Run(func(args mock.Arguments) {
		u := args.Get(1).(*entity.User)
		u.ID = 1
	}).Return(nil)
	mTokenRepo.On("StoreTokenPair", mock.Anything, mock.MatchedBy(func(td *entity.TokenDetails) bool {
		return td.AccessToken != "" && td.RefreshToken != ""
	}), uint(1)).Return(nil)

	body := `{"email":"test@example.com","password":"password123","name":"Test User","whatsapp":"081234567890","role":"customer"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.True(t, apiResp.Success)
	assert.NotNil(t, apiResp.Data)

	dataBytes, _ := json.Marshal(apiResp.Data)
	var authData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		User         struct {
			ID    uint   `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(dataBytes, &authData))
	assert.NotEmpty(t, authData.AccessToken)
	assert.NotEmpty(t, authData.RefreshToken)
	assert.Equal(t, uint(1), authData.User.ID)
	assert.Equal(t, "test@example.com", authData.User.Email)
	assert.Equal(t, "Test User", authData.User.Name)

	mUserRepo.AssertExpectations(t)
	mTokenRepo.AssertExpectations(t)
	mLimiter.AssertExpectations(t)
}

func TestAuthHandler_Register_InvalidEmail_Returns400(t *testing.T) {
	t.Parallel()

	app, _, _, _, _ := setupTestApp(t)

	body := `{"email":"not-an-email","password":"password123","name":"Test User","whatsapp":"081234567890","role":"customer"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "VALIDATION_ERROR", apiResp.Code)
}

func TestAuthHandler_Register_ShortPassword_Returns400(t *testing.T) {
	t.Parallel()

	app, _, _, _, _ := setupTestApp(t)

	body := `{"email":"test@example.com","password":"short","name":"Test User","whatsapp":"081234567890","role":"customer"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "VALIDATION_ERROR", apiResp.Code)
}

func TestAuthHandler_Register_DuplicateEmail_Returns409(t *testing.T) {
	t.Parallel()

	app, mUserRepo, _, mLimiter, _ := setupTestApp(t)

	mUserRepo.On("ExistsByEmail", mock.Anything, "test@example.com").Return(true, nil)

	body := `{"email":"test@example.com","password":"password123","name":"Test User","whatsapp":"081234567890","role":"customer"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "EMAIL_EXISTS", apiResp.Code)

	mUserRepo.AssertExpectations(t)
	mLimiter.AssertExpectations(t)
}

func TestAuthHandler_Login_ValidRequest_Returns200(t *testing.T) {
	t.Parallel()

	app, mUserRepo, mTokenRepo, mLimiter, _ := setupTestApp(t)

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.MinCost)
	require.NoError(t, err)

	existingUser := &entity.User{
		ID:       42,
		Email:    "user@example.com",
		Password: string(hashedPwd),
		Name:     "Existing User",
	}

	mLimiter.On("IsLocked", mock.Anything, "user@example.com").Return(false, nil)
	mUserRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(existingUser, nil)
	mLimiter.On("Reset", mock.Anything, "user@example.com").Return(nil)
	mTokenRepo.On("StoreTokenPair", mock.Anything, mock.MatchedBy(func(td *entity.TokenDetails) bool {
		return td.AccessToken != "" && td.RefreshToken != ""
	}), uint(42)).Return(nil)

	body := `{"identifier":"user@example.com","password":"correctpassword"}`
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.True(t, apiResp.Success)

	dataBytes, _ := json.Marshal(apiResp.Data)
	var authData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID    uint   `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(dataBytes, &authData))
	assert.NotEmpty(t, authData.AccessToken)
	assert.NotEmpty(t, authData.RefreshToken)
	assert.Equal(t, uint(42), authData.User.ID)
	assert.Equal(t, "user@example.com", authData.User.Email)

	mUserRepo.AssertExpectations(t)
	mTokenRepo.AssertExpectations(t)
	mLimiter.AssertExpectations(t)
}

func TestAuthHandler_Login_WrongPassword_Returns401(t *testing.T) {
	t.Parallel()

	app, mUserRepo, _, mLimiter, _ := setupTestApp(t)

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.MinCost)
	require.NoError(t, err)

	existingUser := &entity.User{
		ID:       1,
		Email:    "user@example.com",
		Password: string(hashedPwd),
		Name:     "User",
	}

	mLimiter.On("IsLocked", mock.Anything, "user@example.com").Return(false, nil)
	mUserRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(existingUser, nil)
	mLimiter.On("RecordFailure", mock.Anything, "user@example.com").Return(nil)

	body := `{"identifier":"user@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "INVALID_CREDENTIALS", apiResp.Code)

	mUserRepo.AssertExpectations(t)
	mLimiter.AssertExpectations(t)
}

func TestAuthHandler_Logout_WithBearerToken_Returns200(t *testing.T) {
	t.Parallel()

	app, _, mTokenRepo, _, tokenSvc := setupTestApp(t)

	td, err := tokenSvc.GenerateTokenPair(1, "user@example.com")
	require.NoError(t, err)

	mTokenRepo.On("BlacklistAccessToken", mock.Anything, td.AccessUUID, time.Hour*24).Return(nil)

	req := httptest.NewRequest("POST", "/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+td.AccessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.True(t, apiResp.Success)
	assert.Equal(t, "logged out successfully", apiResp.Message)

	mTokenRepo.AssertExpectations(t)
}

func TestAuthHandler_RefreshToken_ValidRequest_Returns200(t *testing.T) {
	t.Parallel()

	app, mUserRepo, mTokenRepo, _, tokenSvc := setupTestApp(t)

	existingUser := &entity.User{
		ID:       7,
		Email:    "refresh@example.com",
		Password: "hashed",
		Name:     "Refresh User",
	}

	td, err := tokenSvc.GenerateTokenPair(existingUser.ID, existingUser.Email)
	require.NoError(t, err)

	_, refreshUUID, err := tokenSvc.ValidateRefreshToken(td.RefreshToken)
	require.NoError(t, err)

	mTokenRepo.On("IsRefreshTokenValid", mock.Anything, refreshUUID).Return(existingUser.ID, nil)
	mTokenRepo.On("RevokeRefreshToken", mock.Anything, refreshUUID).Return(nil)
	mUserRepo.On("FindByID", mock.Anything, existingUser.ID).Return(existingUser, nil)
	mTokenRepo.On("StoreTokenPair", mock.Anything, mock.MatchedBy(func(newTd *entity.TokenDetails) bool {
		return newTd.AccessToken != "" && newTd.RefreshToken != ""
	}), existingUser.ID).Return(nil)

	body := `{"refresh_token":"` + td.RefreshToken + `"}`
	req := httptest.NewRequest("POST", "/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.True(t, apiResp.Success)

	dataBytes, _ := json.Marshal(apiResp.Data)
	var authData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(dataBytes, &authData))
	assert.NotEmpty(t, authData.AccessToken)
	assert.NotEmpty(t, authData.RefreshToken)

	mTokenRepo.AssertExpectations(t)
	mUserRepo.AssertExpectations(t)
}

func TestAuthHandler_RefreshToken_MissingBody_Returns400(t *testing.T) {
	t.Parallel()

	app, _, _, _, _ := setupTestApp(t)

	req := httptest.NewRequest("POST", "/refresh", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAuthHandler_RefreshToken_MissingRefreshToken_Returns400(t *testing.T) {
	t.Parallel()

	app, _, _, _, _ := setupTestApp(t)

	body := `{}`
	req := httptest.NewRequest("POST", "/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	var apiResp apiErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiResp))
	assert.Equal(t, "VALIDATION_ERROR", apiResp.Code)
}
