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

// F1+F8: Suspend revokes live tokens, refuses self-suspend, and guards the
// last active admin; SetRole guards demoting the last active admin.

func setupGuardUsecase(mockRepo *MockUserRepository, mockTokens *MockTokenRepository) *AdminUserUsecase {
	uc := NewAdminUserUsecase(mockRepo)
	uc.SetTokenRepository(mockTokens)
	return uc
}

func TestAdminUserUsecase_Suspend_RevokesTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	mockTokens := new(MockTokenRepository)
	uc := setupGuardUsecase(mockRepo, mockTokens)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)
	mockRepo.On("UpdateStatus", ctx, uint(3), entity.UserStatusSuspended).Return(nil)
	mockTokens.On("RevokeAllUserTokens", ctx, uint(3)).Return(nil)

	updated, err := uc.Suspend(ctx, 7, 3)

	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, updated.Status)
	mockTokens.AssertExpectations(t)
}

func TestAdminUserUsecase_Suspend_RevocationFailure_StillSuspends(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	mockTokens := new(MockTokenRepository)
	uc := setupGuardUsecase(mockRepo, mockTokens)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)
	mockRepo.On("UpdateStatus", ctx, uint(3), entity.UserStatusSuspended).Return(nil)
	mockTokens.On("RevokeAllUserTokens", ctx, uint(3)).Return(assert.AnError)

	updated, err := uc.Suspend(ctx, 7, 3)

	// Revocation is best-effort: Authenticate rejects suspended accounts
	// regardless, so the suspend itself must succeed.
	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, updated.Status)
}

func TestAdminUserUsecase_Suspend_SelfSuspend_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupGuardUsecase(mockRepo, new(MockTokenRepository))

	updated, err := uc.Suspend(ctx, 7, 7)

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "SELF_SUSPEND", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrForbidden)
	mockRepo.AssertNotCalled(t, "FindByID", ctx, uint(7))
}

func TestAdminUserUsecase_Suspend_LastAdmin_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	mockTokens := new(MockTokenRepository)
	uc := setupGuardUsecase(mockRepo, mockTokens)

	admin := &entity.User{ID: 8, Email: "last@example.com", Role: "admin", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(8)).Return(admin, nil)
	mockRepo.On("CountActiveAdmins", ctx).Return(int64(1), nil)

	updated, err := uc.Suspend(ctx, 7, 8)

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "LAST_ADMIN", domainErr.Code)
	mockRepo.AssertNotCalled(t, "UpdateStatus", ctx, mock.Anything, mock.Anything)
	mockTokens.AssertNotCalled(t, "RevokeAllUserTokens", ctx, mock.Anything)
}

func TestAdminUserUsecase_Suspend_FellowAdminNotLast_Suspends(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	mockTokens := new(MockTokenRepository)
	uc := setupGuardUsecase(mockRepo, mockTokens)

	admin := &entity.User{ID: 8, Email: "other@example.com", Role: "admin", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(8)).Return(admin, nil)
	mockRepo.On("CountActiveAdmins", ctx).Return(int64(2), nil)
	mockRepo.On("UpdateStatus", ctx, uint(8), entity.UserStatusSuspended).Return(nil)
	mockTokens.On("RevokeAllUserTokens", ctx, uint(8)).Return(nil)

	updated, err := uc.Suspend(ctx, 7, 8)

	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, updated.Status)
	mockRepo.AssertExpectations(t)
	mockTokens.AssertExpectations(t)
}

func TestAdminUserUsecase_SetRole_DemoteLastAdmin_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupGuardUsecase(mockRepo, new(MockTokenRepository))

	admin := &entity.User{ID: 8, Email: "last@example.com", Role: "admin", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(8)).Return(admin, nil)
	mockRepo.On("CountActiveAdmins", ctx).Return(int64(1), nil)

	updated, err := uc.SetRole(ctx, 7, 8, "provider")

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "LAST_ADMIN", domainErr.Code)
	mockRepo.AssertNotCalled(t, "UpdateRole", ctx, mock.Anything, mock.Anything)
}

func TestAdminUserUsecase_SetRole_DemoteFellowAdminNotLast_Ok(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupGuardUsecase(mockRepo, new(MockTokenRepository))

	admin := &entity.User{ID: 8, Email: "other@example.com", Role: "admin", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(8)).Return(admin, nil)
	mockRepo.On("CountActiveAdmins", ctx).Return(int64(3), nil)
	mockRepo.On("UpdateRole", ctx, uint(8), "provider").Return(nil)

	updated, err := uc.SetRole(ctx, 7, 8, "provider")

	require.NoError(t, err)
	assert.Equal(t, "provider", updated.Role)
	mockRepo.AssertExpectations(t)
}
