package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
)

// Ticket 8: introspect signals users.chat_blocked so the chat backend can
// refuse token exchange; the signal is additive (token-validity semantics
// unchanged) and lookup failures fail open.

func setupIntrospectUsecase() (*IntrospectUsecase, *MockUserRepository, *MockTokenRepository, *service.TokenService) {
	tokenSvc := service.NewTokenService("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	mockTokenRepo := new(MockTokenRepository)
	mockUserRepo := new(MockUserRepository)
	return NewIntrospectUsecase(tokenSvc, mockTokenRepo, mockUserRepo), mockUserRepo, mockTokenRepo, tokenSvc
}

func mintAccessToken(t *testing.T, tokenSvc *service.TokenService, userID uint) string {
	t.Helper()
	td, err := tokenSvc.GenerateTokenPair(userID, "user@example.com")
	require.NoError(t, err)
	return td.AccessToken
}

func TestIntrospectUsecase_SignalsChatBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, mockUserRepo, mockTokenRepo, tokenSvc := setupIntrospectUsecase()
	token := mintAccessToken(t, tokenSvc, 5)

	mockTokenRepo.On("IsAccessTokenBlacklisted", ctx, mock.Anything).Return(false, nil)
	mockUserRepo.On("FindByID", ctx, uint(5)).Return(&entity.User{ID: 5, ChatBlocked: true}, nil)

	result := uc.Introspect(ctx, token)

	assert.True(t, result.Active)
	assert.Equal(t, uint(5), result.UserID)
	assert.True(t, result.ChatBlocked)
	mockUserRepo.AssertExpectations(t)
}

func TestIntrospectUsecase_UnblockedUser_SignalsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, mockUserRepo, mockTokenRepo, tokenSvc := setupIntrospectUsecase()
	token := mintAccessToken(t, tokenSvc, 5)

	mockTokenRepo.On("IsAccessTokenBlacklisted", ctx, mock.Anything).Return(false, nil)
	mockUserRepo.On("FindByID", ctx, uint(5)).Return(&entity.User{ID: 5, ChatBlocked: false}, nil)

	result := uc.Introspect(ctx, token)

	assert.True(t, result.Active)
	assert.False(t, result.ChatBlocked)
}

func TestIntrospectUsecase_LookupFailure_FailsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, mockUserRepo, mockTokenRepo, tokenSvc := setupIntrospectUsecase()
	token := mintAccessToken(t, tokenSvc, 5)

	mockTokenRepo.On("IsAccessTokenBlacklisted", ctx, mock.Anything).Return(false, nil)
	mockUserRepo.On("FindByID", ctx, uint(5)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	result := uc.Introspect(ctx, token)

	assert.True(t, result.Active)
	assert.False(t, result.ChatBlocked)
}

func TestIntrospectUsecase_InvalidToken_StaysInactive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, _, _, _ := setupIntrospectUsecase()

	result := uc.Introspect(ctx, "not-a-token")

	assert.False(t, result.Active)
}
