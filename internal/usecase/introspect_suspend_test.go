package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// F2: introspect reports Active:false for suspended users so downstream
// consumers (chat token exchange) refuse fresh mints for them.

func TestIntrospectUsecase_SuspendedUser_ReportsInactive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, mockUserRepo, mockTokenRepo, tokenSvc := setupIntrospectUsecase()
	token := mintAccessToken(t, tokenSvc, 9)

	mockTokenRepo.On("IsAccessTokenBlacklisted", ctx, mock.Anything).Return(false, nil)
	mockUserRepo.On("FindByID", ctx, uint(9)).Return(&entity.User{
		ID: 9, Status: entity.UserStatusSuspended, ChatBlocked: false,
	}, nil)

	result := uc.Introspect(ctx, token)

	assert.False(t, result.Active)
	assert.Equal(t, uint(0), result.UserID)
	mockUserRepo.AssertExpectations(t)
}

func TestIntrospectUsecase_ActiveUser_ReportsActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	uc, mockUserRepo, mockTokenRepo, tokenSvc := setupIntrospectUsecase()
	token := mintAccessToken(t, tokenSvc, 9)

	mockTokenRepo.On("IsAccessTokenBlacklisted", ctx, mock.Anything).Return(false, nil)
	mockUserRepo.On("FindByID", ctx, uint(9)).Return(&entity.User{
		ID: 9, Status: entity.UserStatusActive, ChatBlocked: false,
	}, nil)

	result := uc.Introspect(ctx, token)

	assert.True(t, result.Active)
	assert.Equal(t, uint(9), result.UserID)
	assert.False(t, result.ChatBlocked)
}
