package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/chatclient"
)

// Ticket 8: chat safety — chat-block toggle idempotent, metadata proxy,
// no message-content fields anywhere in the contract.

type fakeChatDirectory struct {
	rows  []chatclient.ConversationMeta
	total int64
	err   error

	lastUserID uint
	lastPage   int
	lastLimit  int
}

func (f *fakeChatDirectory) ListConversations(ctx context.Context, userID uint, page, limit int) ([]chatclient.ConversationMeta, int64, error) {
	f.lastUserID, f.lastPage, f.lastLimit = userID, page, limit
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.rows, f.total, nil
}

func TestAdminChatUsecase_SetChatBlock_Blocks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, &fakeChatDirectory{})

	user := &entity.User{ID: 5, Email: "spam@example.com", Role: "customer", Status: entity.UserStatusActive}
	mockRepo.On("FindByID", ctx, uint(5)).Return(user, nil)
	mockRepo.On("UpdateChatBlocked", ctx, uint(5), true).Return(nil)

	updated, err := uc.SetChatBlock(ctx, 5, true)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, updated.ChatBlocked)
	assert.Equal(t, entity.UserStatusActive, updated.Status)
	mockRepo.AssertExpectations(t)
}

func TestAdminChatUsecase_SetChatBlock_Unblocks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, &fakeChatDirectory{})

	user := &entity.User{ID: 5, Email: "spam@example.com", Status: entity.UserStatusActive, ChatBlocked: true}
	mockRepo.On("FindByID", ctx, uint(5)).Return(user, nil)
	mockRepo.On("UpdateChatBlocked", ctx, uint(5), false).Return(nil)

	updated, err := uc.SetChatBlock(ctx, 5, false)

	require.NoError(t, err)
	assert.False(t, updated.ChatBlocked)
	mockRepo.AssertExpectations(t)
}

func TestAdminChatUsecase_SetChatBlock_AlreadyBlocked_IsNoopSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, &fakeChatDirectory{})

	user := &entity.User{ID: 5, Email: "spam@example.com", Status: entity.UserStatusActive, ChatBlocked: true}
	mockRepo.On("FindByID", ctx, uint(5)).Return(user, nil)

	updated, err := uc.SetChatBlock(ctx, 5, true)

	require.NoError(t, err)
	assert.True(t, updated.ChatBlocked)
	mockRepo.AssertNotCalled(t, "UpdateChatBlocked", ctx, uint(5), true)
}

func TestAdminChatUsecase_SetChatBlock_UserNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, &fakeChatDirectory{})

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	updated, err := uc.SetChatBlock(ctx, 99, true)

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "USER_NOT_FOUND", domainErr.Code)
}

func TestAdminChatUsecase_ListConversations_ReturnsMetadataOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	dir := &fakeChatDirectory{
		rows: []chatclient.ConversationMeta{
			{ConversationID: "11111111-1111-1111-1111-111111111111", UserAID: 5, UserBID: 9, MessageCount: 42, LastActivityAt: "2026-10-14T12:00:00Z"},
		},
		total: 1,
	}
	uc := NewAdminChatUsecase(mockRepo, dir)

	rows, total, err := uc.ListConversations(ctx, 5, 1, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", rows[0].ConversationID)
	assert.Equal(t, uint(5), rows[0].UserAID)
	assert.Equal(t, uint(9), rows[0].UserBID)
	assert.Equal(t, int64(42), rows[0].MessageCount)
	assert.Equal(t, "2026-10-14T12:00:00Z", rows[0].LastActivityAt)
	assert.Equal(t, int64(0), rows[0].Flags)
	assert.Equal(t, uint(5), dir.lastUserID)
	assert.Equal(t, 1, dir.lastPage)
	assert.Equal(t, 20, dir.lastLimit)
}

func TestAdminChatUsecase_ListConversations_DirectoryError_MapsToUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, &fakeChatDirectory{err: assert.AnError})

	rows, _, err := uc.ListConversations(ctx, 0, 1, 20)

	require.Error(t, err)
	assert.Nil(t, rows)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "CHAT_DIRECTORY_UNAVAILABLE", domainErr.Code)
}

func TestAdminChatUsecase_ListConversations_Unconfigured_MapsToUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockUserRepository)
	uc := NewAdminChatUsecase(mockRepo, nil)

	rows, _, err := uc.ListConversations(ctx, 0, 1, 20)

	require.Error(t, err)
	assert.Nil(t, rows)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "CHAT_DIRECTORY_UNAVAILABLE", domainErr.Code)
}
