package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Ticket 3: admin user management — suspend/restore idempotent, role change
// with self-demote guard, list with search + filters.

func setupAdminUserUsecase(mockRepo *MockUserRepository) *AdminUserUsecase {
	return NewAdminUserUsecase(mockRepo)
}

func TestAdminUserUsecase_Suspend_SetsSuspended(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)
	mockRepo.On("UpdateStatus", ctx, uint(3), entity.UserStatusSuspended).Return(nil)

	updated, err := uc.Suspend(ctx, 7, 3)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, entity.UserStatusSuspended, updated.Status)
	mockRepo.AssertExpectations(t)
}

func TestAdminUserUsecase_Suspend_AlreadySuspended_IsNoopSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusSuspended}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)

	updated, err := uc.Suspend(ctx, 7, 3)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, entity.UserStatusSuspended, updated.Status)
	mockRepo.AssertNotCalled(t, "UpdateStatus", ctx, uint(3), entity.UserStatusSuspended)
}

func TestAdminUserUsecase_Suspend_UserNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	updated, err := uc.Suspend(ctx, 7, 99)

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "USER_NOT_FOUND", domainErr.Code)
}

func TestAdminUserUsecase_Restore_SetsActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusSuspended}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)
	mockRepo.On("UpdateStatus", ctx, uint(3), entity.UserStatusActive).Return(nil)

	updated, err := uc.Restore(ctx, 3)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, entity.UserStatusActive, updated.Status)
	mockRepo.AssertExpectations(t)
}

func TestAdminUserUsecase_Restore_AlreadyActive_IsNoopSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)

	updated, err := uc.Restore(ctx, 3)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, entity.UserStatusActive, updated.Status)
	mockRepo.AssertNotCalled(t, "UpdateStatus", ctx, uint(3), entity.UserStatusActive)
}

func TestAdminUserUsecase_SetRole_PromotesCustomerToProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	user := &entity.User{ID: 3, Email: "budi@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(3)).Return(user, nil)
	mockRepo.On("UpdateRole", ctx, uint(3), "provider").Return(nil)

	updated, err := uc.SetRole(ctx, 7, 3, "provider")

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "provider", updated.Role)
	mockRepo.AssertExpectations(t)
}

func TestAdminUserUsecase_SetRole_InvalidRole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	updated, err := uc.SetRole(ctx, 7, 3, "superadmin")

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "VALIDATION_ERROR", domainErr.Code)
	mockRepo.AssertNotCalled(t, "FindByID", ctx, uint(3))
}

func TestAdminUserUsecase_SetRole_SelfDemote_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	admin := &entity.User{ID: 7, Email: "admin@example.com", Role: "admin", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(7)).Return(admin, nil)

	updated, err := uc.SetRole(ctx, 7, 7, "customer")

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "SELF_DEMOTE", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrForbidden)
	mockRepo.AssertNotCalled(t, "UpdateRole", ctx, uint(7), "customer")
}

func TestAdminUserUsecase_SetRole_UserNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	updated, err := uc.SetRole(ctx, 7, 99, "provider")

	require.Error(t, err)
	assert.Nil(t, updated)
}

func TestAdminUserUsecase_List_PassesFilterThrough(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	rows := []*entity.User{
		{ID: 3, Email: "budi@example.com", Role: "provider", Status: entity.UserStatusActive},
	}
	filter := repository.UserFilter{Q: "budi", Role: "provider", Status: "active", Page: 1, Limit: 20}
	mockRepo.On("List", ctx, filter).Return(rows, int64(1), nil)

	got, total, err := uc.ListUsers(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	assert.Equal(t, "budi@example.com", got[0].Email)
	mockRepo.AssertExpectations(t)
}

func TestAdminUserUsecase_GetUser_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := setupAdminUserUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	got, err := uc.GetUser(ctx, 99)

	require.Error(t, err)
	assert.Nil(t, got)
}
