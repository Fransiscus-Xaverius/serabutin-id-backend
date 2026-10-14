package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

// Ticket 1: suspended users cannot log in (403 ACCOUNT_SUSPENDED).

func TestAuthUsecase_Login_SuspendedUser_ReturnsAccountSuspended(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	suspendedUser := &entity.User{
		ID:       9,
		Email:    "suspended@example.com",
		Password: hashPassword(t, "correctpassword"),
		Name:     "Suspended User",
		Status:   entity.UserStatusSuspended,
	}

	mockLimiter.On("IsLocked", ctx, "suspended@example.com").Return(false, nil)
	mockUserRepo.On("FindByEmail", ctx, "suspended@example.com").Return(suspendedUser, nil)

	td, user, err := uc.Login(ctx, "suspended@example.com", "correctpassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "ACCOUNT_SUSPENDED", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrAccountSuspended)
	assert.Nil(t, td)
	assert.Nil(t, user)

	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
	mockLimiter.AssertNotCalled(t, "RecordFailure")
	mockLimiter.AssertNotCalled(t, "Reset")
}

func TestAuthUsecase_Login_SuspendedUserViaWhatsApp_ReturnsAccountSuspended(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	suspendedUser := &entity.User{
		ID:       10,
		Email:    "wa-suspended@example.com",
		Password: hashPassword(t, "correctpassword"),
		Name:     "WA Suspended",
		WhatsApp: "081234567890",
		Status:   entity.UserStatusSuspended,
	}

	mockLimiter.On("IsLocked", ctx, "081234567890").Return(false, nil)
	mockUserRepo.On("FindByWhatsApp", ctx, "081234567890").Return(suspendedUser, nil)

	td, user, err := uc.Login(ctx, "081234567890", "correctpassword")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "ACCOUNT_SUSPENDED", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrAccountSuspended)
	assert.Nil(t, td)
	assert.Nil(t, user)

	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
}

func TestAuthUsecase_Login_ActiveUser_Success(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, mockLimiter, _ := setupAuthUsecase(t)
	ctx := context.Background()

	activeUser := &entity.User{
		ID:       11,
		Email:    "active@example.com",
		Password: hashPassword(t, "correctpassword"),
		Name:     "Active User",
		Status:   entity.UserStatusActive,
	}

	mockLimiter.On("IsLocked", ctx, "active@example.com").Return(false, nil)
	mockUserRepo.On("FindByEmail", ctx, "active@example.com").Return(activeUser, nil)
	mockLimiter.On("Reset", ctx, "active@example.com").Return(nil)
	mockTokenRepo.On("StoreTokenPair", ctx, mock.MatchedBy(func(td *entity.TokenDetails) bool {
		return td.AccessToken != "" && td.RefreshToken != ""
	}), uint(11)).Return(nil)

	td, user, err := uc.Login(ctx, "active@example.com", "correctpassword")

	require.NoError(t, err)
	require.NotNil(t, td)
	require.NotNil(t, user)
	assert.Equal(t, uint(11), user.ID)

	mockUserRepo.AssertExpectations(t)
	mockTokenRepo.AssertExpectations(t)
	mockLimiter.AssertExpectations(t)
}

func TestAuthUsecase_RefreshToken_SuspendedUser_ReturnsAccountSuspended(t *testing.T) {
	t.Parallel()

	uc, mockUserRepo, mockTokenRepo, _, tokenSvc := setupAuthUsecase(t)
	ctx := context.Background()

	suspendedUser := &entity.User{
		ID:       12,
		Email:    "refresh-suspended@example.com",
		Password: hashPassword(t, "password"),
		Name:     "Refresh Suspended",
		Status:   entity.UserStatusSuspended,
	}

	td, err := tokenSvc.GenerateTokenPair(suspendedUser.ID, suspendedUser.Email)
	require.NoError(t, err)

	_, refreshUUID, err := tokenSvc.ValidateRefreshToken(td.RefreshToken)
	require.NoError(t, err)

	mockTokenRepo.On("IsRefreshTokenValid", ctx, refreshUUID).Return(suspendedUser.ID, nil)
	mockTokenRepo.On("RevokeRefreshToken", ctx, refreshUUID).Return(nil)
	mockUserRepo.On("FindByID", ctx, suspendedUser.ID).Return(suspendedUser, nil)

	newTd, err := uc.RefreshToken(ctx, td.RefreshToken)

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "ACCOUNT_SUSPENDED", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrAccountSuspended)
	assert.Nil(t, newTd)

	mockTokenRepo.AssertNotCalled(t, "StoreTokenPair")
}
