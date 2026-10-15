package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
)

type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepository) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	args := m.Called(ctx, whatsapp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.User), args.Get(1).(int64), args.Error(2)
}

func (m *MockUserRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockUserRepository) UpdateRole(ctx context.Context, id uint, role string) error {
	args := m.Called(ctx, id, role)
	return args.Error(0)
}

func (m *MockUserRepository) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	args := m.Called(ctx, id, blocked)
	return args.Error(0)
}

func (m *MockUserRepository) CountActiveAdmins(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

type MockTokenRepository struct {
	mock.Mock
}

func (m *MockTokenRepository) StoreRefreshToken(ctx context.Context, userID uint, tokenUUID string, expires time.Duration) error {
	args := m.Called(ctx, userID, tokenUUID, expires)
	return args.Error(0)
}

func (m *MockTokenRepository) IsRefreshTokenValid(ctx context.Context, tokenUUID string) (uint, error) {
	args := m.Called(ctx, tokenUUID)
	return args.Get(0).(uint), args.Error(1)
}

func (m *MockTokenRepository) RevokeRefreshToken(ctx context.Context, tokenUUID string) error {
	args := m.Called(ctx, tokenUUID)
	return args.Error(0)
}

func (m *MockTokenRepository) RevokeAllUserTokens(ctx context.Context, userID uint) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockTokenRepository) BlacklistAccessToken(ctx context.Context, tokenUUID string, expires time.Duration) error {
	args := m.Called(ctx, tokenUUID, expires)
	return args.Error(0)
}

func (m *MockTokenRepository) IsAccessTokenBlacklisted(ctx context.Context, tokenUUID string) (bool, error) {
	args := m.Called(ctx, tokenUUID)
	return args.Bool(0), args.Error(1)
}

func (m *MockTokenRepository) StoreTokenPair(ctx context.Context, td *entity.TokenDetails, userID uint) error {
	args := m.Called(ctx, td, userID)
	return args.Error(0)
}

type MockLoginLimiter struct {
	mock.Mock
}

func (m *MockLoginLimiter) IsLocked(ctx context.Context, identifier string) (bool, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Error(1)
}

func (m *MockLoginLimiter) RecordFailure(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

func (m *MockLoginLimiter) Reset(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

func (m *MockLoginLimiter) RemainingAttempts(ctx context.Context, identifier string) (int, error) {
	args := m.Called(ctx, identifier)
	return args.Int(0), args.Error(1)
}

func setupAuthUsecase(t *testing.T) (*AuthUsecase, *MockUserRepository, *MockTokenRepository, *MockLoginLimiter, *service.TokenService) {
	t.Helper()

	mockUserRepo := new(MockUserRepository)
	mockTokenRepo := new(MockTokenRepository)
	mockLimiter := new(MockLoginLimiter)
	tokenSvc := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)

	uc := NewAuthUsecase(mockUserRepo, mockTokenRepo, tokenSvc, mockLimiter, bcrypt.MinCost)

	return uc, mockUserRepo, mockTokenRepo, mockLimiter, tokenSvc
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(hashed)
}

func TestAuthUsecase_Register_Success(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	mockUserRepo.On("ExistsByEmail", ctx, "test@example.com").Return(false, nil)
	mockUserRepo.On("FindByWhatsApp", ctx, "081234567890").Return((*entity.User)(nil), apperror.ErrNotFound)
	mockUserRepo.On("Create", ctx, mock.MatchedBy(func(u *entity.User) bool {
		return u.Email == "test@example.com" && u.Name == "Test User" && u.WhatsApp == "081234567890" && u.Role == "customer" && u.ConsentAt != nil
	})).Run(func(args mock.Arguments) {
		u := args.Get(1).(*entity.User)
		u.ID = 1
	}).Return(nil)
	mockTokenRepo.On("StoreTokenPair", ctx, mock.MatchedBy(func(td *entity.TokenDetails) bool {
		return td.AccessToken != "" && td.RefreshToken != ""
	}), uint(1)).Return(nil)

	td, user, err := uc.Register(ctx, "test@example.com", "password123", "Test User", "081234567890", "customer")

	require.NoError(t, err)
	require.NotNil(t, td)
	assert.NotEmpty(t, td.AccessToken)
	assert.NotEmpty(t, td.RefreshToken)
	assert.NotEmpty(t, td.AccessUUID)
	assert.NotEmpty(t, td.RefreshUUID)
	require.NotNil(t, user)
	assert.Equal(t, uint(1), user.ID)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "081234567890", user.WhatsApp)
	assert.Equal(t, "customer", user.Role)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertExpectations(t)
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_Register_DuplicateEmail_ReturnsErrDuplicateEmail(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	mockUserRepo.On("ExistsByEmail", ctx, "test@example.com").Return(true, nil)

	td, user, err := uc.Register(ctx, "test@example.com", "password123", "Test User", "081234567890", "customer")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "EMAIL_EXISTS", domainErr.Code)
	assert.True(t, errors.Is(domainErr.Err, apperror.ErrDuplicateEmail))
	assert.Nil(t, td)
	assert.Nil(t, user)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_Success(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	hashedPwd := hashPassword(t, "correctpassword")
	existingUser := &entity.User{
		ID:       42,
		Email:    "user@example.com",
		Password: hashedPwd,
		Name:     "Existing User",
	}

	mockLimiter.On("IsLocked", ctx, "user@example.com").Return(false, nil)
	mockUserRepo.On("FindByEmail", ctx, "user@example.com").Return(existingUser, nil)
	mockLimiter.On("Reset", ctx, "user@example.com").Return(nil)
	mockTokenRepo.On("StoreTokenPair", ctx, mock.MatchedBy(func(td *entity.TokenDetails) bool {
		return td.AccessToken != "" && td.RefreshToken != ""
	}), uint(42)).Return(nil)

	td, user, err := uc.Login(ctx, "user@example.com", "correctpassword")

	require.NoError(t, err)
	require.NotNil(t, td)
	assert.NotEmpty(t, td.AccessToken)
	assert.NotEmpty(t, td.RefreshToken)
	require.NotNil(t, user)
	assert.Equal(t, uint(42), user.ID)
	assert.Equal(t, "user@example.com", user.Email)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertExpectations(t)
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	hashedPwd := hashPassword(t, "correctpassword")
	existingUser := &entity.User{
		ID:       1,
		Email:    "user@example.com",
		Password: hashedPwd,
		Name:     "User",
	}

	mockLimiter.On("IsLocked", ctx, "user@example.com").Return(false, nil)
	mockUserRepo.On("FindByEmail", ctx, "user@example.com").Return(existingUser, nil)
	mockLimiter.On("RecordFailure", ctx, "user@example.com").Return(nil)

	td, user, err := uc.Login(ctx, "user@example.com", "wrongpassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_CREDENTIALS", domainErr.Code)
	assert.True(t, errors.Is(domainErr.Err, apperror.ErrInvalidCredentials))
	assert.Nil(t, td)
	assert.Nil(t, user)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_NonExistentEmail_ReturnsErrInvalidCredentials(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	mockLimiter.On("IsLocked", ctx, "nonexistent@example.com").Return(false, nil)
	mockUserRepo.On("FindByEmail", ctx, "nonexistent@example.com").Return((*entity.User)(nil), apperror.ErrNotFound)
	mockLimiter.On("RecordFailure", ctx, "nonexistent@example.com").Return(nil)

	td, user, err := uc.Login(ctx, "nonexistent@example.com", "anypassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_CREDENTIALS", domainErr.Code)
	assert.True(t, errors.Is(domainErr.Err, apperror.ErrInvalidCredentials))
	assert.Nil(t, td)
	assert.Nil(t, user)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_Logout_BlacklistsAccessToken(t *testing.T) {
	t.Parallel()

	uc, _, mockTokenRepo, _, tokenSvc := setupAuthUsecase(t)
	ctx := context.Background()

	td, err := tokenSvc.GenerateTokenPair(1, "user@example.com")
	require.NoError(t, err)

	mockTokenRepo.On("BlacklistAccessToken", ctx, td.AccessUUID, time.Hour*24).Return(nil)

	err = uc.Logout(ctx, td.AccessToken, "")

	require.NoError(t, err)
	mockTokenRepo.AssertCalled(t, "BlacklistAccessToken", ctx, td.AccessUUID, time.Hour*24)
}

func TestAuthUsecase_Logout_RevokesRefreshToken(t *testing.T) {
	t.Parallel()

	uc, _, mockTokenRepo, _, tokenSvc := setupAuthUsecase(t)
	ctx := context.Background()

	td, err := tokenSvc.GenerateTokenPair(1, "user@example.com")
	require.NoError(t, err)

	mockTokenRepo.On("RevokeRefreshToken", ctx, td.RefreshUUID).Return(nil)

	err = uc.Logout(ctx, "", td.RefreshToken)

	require.NoError(t, err)
	mockTokenRepo.AssertCalled(t, "RevokeRefreshToken", ctx, td.RefreshUUID)
}

func TestAuthUsecase_Logout_EmptyTokens_DoesNothing(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, _, _ := setupAuthUsecase(t)
	ctx := context.Background()

	err := uc.Logout(ctx, "", "")

	require.NoError(t, err)
	mockUserRepo.AssertNotCalled(t, "ExistsByEmail")
	mockUserRepo.AssertNotCalled(t, "FindByEmail")
	mockUserRepo.AssertNotCalled(t, "FindByID")
	mockUserRepo.AssertNotCalled(t, "Create")
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
	mockTokenRepo.AssertNotCalled(t, "BlacklistAccessToken")
	mockTokenRepo.AssertNotCalled(t, "RevokeRefreshToken")
	mockTokenRepo.AssertNotCalled(t, "IsRefreshTokenValid")
}

func TestAuthUsecase_RefreshToken_Success(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, _, tokenSvc := setupAuthUsecase(t)
	ctx := context.Background()

	existingUser := &entity.User{
		ID:       7,
		Email:    "refresh@example.com",
		Password: hashPassword(t, "password"),
		Name:     "Refresh User",
	}

	td, err := tokenSvc.GenerateTokenPair(existingUser.ID, existingUser.Email)
	require.NoError(t, err)

	_, refreshUUID, err := tokenSvc.ValidateRefreshToken(td.RefreshToken)
	require.NoError(t, err)

	mockTokenRepo.On("IsRefreshTokenValid", ctx, refreshUUID).Return(existingUser.ID, nil)
	mockTokenRepo.On("RevokeRefreshToken", ctx, refreshUUID).Return(nil)
	mockUserRepo.On("FindByID", ctx, existingUser.ID).Return(existingUser, nil)
	mockTokenRepo.On("StoreTokenPair", ctx, mock.MatchedBy(func(newTd *entity.TokenDetails) bool {
		return newTd.AccessToken != "" && newTd.RefreshToken != ""
	}), existingUser.ID).Return(nil)

	newTd, err := uc.RefreshToken(ctx, td.RefreshToken)

	require.NoError(t, err)
	require.NotNil(t, newTd)
	assert.NotEmpty(t, newTd.AccessToken)
	assert.NotEmpty(t, newTd.RefreshToken)
	assert.NotEqual(t, td.AccessToken, newTd.AccessToken, "should generate new access token")
	assert.NotEqual(t, td.RefreshToken, newTd.RefreshToken, "should generate new refresh token")

	mockTokenRepo.AssertCalled(t, "RevokeRefreshToken", ctx, refreshUUID)
	mockUserRepo.AssertCalled(t, "FindByID", ctx, existingUser.ID)
	mockTokenRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

func TestAuthUsecase_RefreshToken_InvalidToken_ReturnsError(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, _, _ := setupAuthUsecase(t)
	ctx := context.Background()

	_, err := uc.RefreshToken(ctx, "not-a-valid-jwt-token")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_REFRESH_TOKEN", domainErr.Code)

	mockUserRepo.AssertNotCalled(t, "FindByID")
	mockTokenRepo.AssertNotCalled(t, "IsRefreshTokenValid")
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
}

func TestAuthUsecase_RefreshToken_RevokedToken_ReturnsError(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, _, tokenSvc := setupAuthUsecase(t)
	ctx := context.Background()

	td, err := tokenSvc.GenerateTokenPair(1, "user@example.com")
	require.NoError(t, err)

	_, refreshUUID, err := tokenSvc.ValidateRefreshToken(td.RefreshToken)
	require.NoError(t, err)

	mockTokenRepo.On("IsRefreshTokenValid", ctx, refreshUUID).Return(uint(0), apperror.ErrTokenRevoked)

	_, err = uc.RefreshToken(ctx, td.RefreshToken)

	require.Error(t, err)
	assert.True(t, errors.Is(err, apperror.ErrTokenRevoked))

	mockUserRepo.AssertNotCalled(t, "FindByID")
	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
}

func TestAuthUsecase_Register_WithWhatsAppAndRole_Success(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	limiter := new(MockLoginLimiter)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, limiter, bcrypt.MinCost)

	userRepo.On("ExistsByEmail", mock.Anything, "test@example.com").Return(false, nil)
	userRepo.On("FindByWhatsApp", mock.Anything, "081234567890").Return((*entity.User)(nil), apperror.ErrNotFound)
	userRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *entity.User) bool {
		return u.Email == "test@example.com" && u.WhatsApp == "081234567890" && u.Role == "customer" && u.ConsentAt != nil
	})).Return(nil).Run(func(args mock.Arguments) {
		u := args.Get(1).(*entity.User)
		u.ID = 1
		u.CreatedAt = time.Now()
		u.UpdatedAt = time.Now()
	})
	tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	td, user, err := uc.Register(context.Background(), "test@example.com", "password123", "Test User", "081234567890", "customer")

	require.NoError(t, err)
	assert.NotNil(t, td)
	assert.Equal(t, "081234567890", user.WhatsApp)
	assert.Equal(t, "customer", user.Role)
	userRepo.AssertExpectations(t)
}

func TestAuthUsecase_Login_WithIdentifierEmail_Success(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	limiter := new(MockLoginLimiter)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, limiter, bcrypt.MinCost)

	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	existingUser := &entity.User{
		ID: 1, Email: "test@example.com", Password: string(hashedPw), Name: "Test User",
		WhatsApp: "081234567890", Role: "customer",
	}
	limiter.On("IsLocked", mock.Anything, "test@example.com").Return(false, nil)
	userRepo.On("FindByEmail", mock.Anything, "test@example.com").Return(existingUser, nil)
	limiter.On("Reset", mock.Anything, "test@example.com").Return(nil)
	tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	td, user, err := uc.Login(context.Background(), "test@example.com", "password123")

	require.NoError(t, err)
	assert.NotNil(t, td)
	assert.Equal(t, "Test User", user.Name)
	userRepo.AssertExpectations(t)
	limiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_WithIdentifierWhatsApp_Success(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	limiter := new(MockLoginLimiter)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, limiter, bcrypt.MinCost)

	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	existingUser := &entity.User{
		ID: 1, Email: "wa@example.com", Password: string(hashedPw), Name: "WA User",
		WhatsApp: "081234567890", Role: "customer",
	}
	limiter.On("IsLocked", mock.Anything, "081234567890").Return(false, nil)
	userRepo.On("FindByWhatsApp", mock.Anything, "081234567890").Return(existingUser, nil)
	limiter.On("Reset", mock.Anything, "081234567890").Return(nil)
	tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	td, user, err := uc.Login(context.Background(), "081234567890", "password123")

	require.NoError(t, err)
	assert.NotNil(t, td)
	assert.Equal(t, "WA User", user.Name)
	userRepo.AssertExpectations(t)
	limiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_WrongPassword_RecordsFailure(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	limiter := new(MockLoginLimiter)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, limiter, bcrypt.MinCost)

	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.MinCost)
	existingUser := &entity.User{
		ID: 1, Email: "test@example.com", Password: string(hashedPw), Name: "Test",
		WhatsApp: "081234567890", Role: "customer",
	}
	limiter.On("IsLocked", mock.Anything, "test@example.com").Return(false, nil)
	userRepo.On("FindByEmail", mock.Anything, "test@example.com").Return(existingUser, nil)
	limiter.On("RecordFailure", mock.Anything, "test@example.com").Return(nil)

	_, _, err := uc.Login(context.Background(), "test@example.com", "wrongpassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrInvalidCredentials, domainErr.Err)
	limiter.AssertExpectations(t)
}

func TestAuthUsecase_Login_Locked_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	limiter := new(MockLoginLimiter)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, limiter, bcrypt.MinCost)

	limiter.On("IsLocked", mock.Anything, "test@example.com").Return(true, nil)

	_, _, err := uc.Login(context.Background(), "test@example.com", "anypassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrInvalidCredentials, domainErr.Err)
	userRepo.AssertNotCalled(t, "FindByEmail")
	userRepo.AssertNotCalled(t, "FindByWhatsApp")
}
