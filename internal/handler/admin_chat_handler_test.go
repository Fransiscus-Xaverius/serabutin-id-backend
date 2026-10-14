package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/chatclient"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 8: admin chat-safety endpoints — metadata-only list, mandatory
// reason + blocked on the block action.

type stubChatDirectory struct {
	rows  []chatclient.ConversationMeta
	total int64
	err   error

	lastUserID uint
}

func (s *stubChatDirectory) ListConversations(_ context.Context, userID uint, _, _ int) ([]chatclient.ConversationMeta, int64, error) {
	s.lastUserID = userID
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.rows, s.total, nil
}

func newAdminChatTestApp(t *testing.T, repo repository.UserRepository, dir usecase.ChatDirectory) *fiber.App {
	t.Helper()
	chatUC := usecase.NewAdminChatUsecase(repo, dir)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminChatUC(chatUC)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/chat/conversations", h.ListChatConversations)
	app.Patch("/api/v1/admin/users/:id/chat-block", h.SetChatBlock)
	return app
}

func TestAdminHandler_ListChatConversations_ReturnsMetadataWithoutContent(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	dir := &stubChatDirectory{
		rows: []chatclient.ConversationMeta{
			{ConversationID: "c-1", UserAID: 5, UserBID: 9, MessageCount: 42, LastActivityAt: "2026-10-14T12:00:00Z"},
		},
		total: 1,
	}
	app := newAdminChatTestApp(t, repo, dir)

	status, body := doAdminUserRequest(t, app, "GET", "/api/v1/admin/chat/conversations?user_id=5&page=1&limit=20", "")

	require.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, uint(5), dir.lastUserID)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 1)
	row := rows[0].(map[string]interface{})
	assert.Equal(t, "c-1", row["conversation_id"])
	assert.Equal(t, float64(5), row["user_a_id"])
	assert.Equal(t, float64(9), row["user_b_id"])
	assert.Equal(t, float64(42), row["message_count"])
	assert.Equal(t, "2026-10-14T12:00:00Z", row["last_activity_at"])
	assert.Equal(t, float64(0), row["flags"])
	// v1 scope: no message-content fields may leak through the proxy.
	raw, _ := json.Marshal(body)
	for _, forbidden := range []string{`"body"`, `"preview"`, `"content"`, `"message_text"`, `"text"`} {
		assert.NotContains(t, string(raw), forbidden, "response must not contain message content")
	}
	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(1), pagination["total"])
}

func TestAdminHandler_ListChatConversations_DirectoryDown_502(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	dir := &stubChatDirectory{err: apperror.NewDomainError(apperror.ErrInternalServer, "CHAT_DIRECTORY_UNAVAILABLE", "chat directory is unavailable")}
	app := newAdminChatTestApp(t, repo, dir)

	status, resp := doAdminUserRequest(t, app, "GET", "/api/v1/admin/chat/conversations", "")

	assert.Equal(t, fiber.StatusBadGateway, status)
	assert.Equal(t, "CHAT_DIRECTORY_UNAVAILABLE", resp["code"])
}

func TestAdminHandler_SetChatBlock_MissingReason_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	for _, body := range []string{"", "{}", `{"blocked":true}`, `{"blocked":true,"reason":""}`} {
		status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/5/chat-block", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetChatBlock_MissingBlocked_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/5/chat-block", `{"reason":"spam link di chat"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
	assert.Contains(t, strings.ToLower(resp["message"].(string)), "blocked")
	repo.AssertNotCalled(t, "FindByID")
}

func TestAdminHandler_SetChatBlock_BlocksWithReason(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(5)).Return(&entity.User{ID: 5, Email: "spam@example.com", Role: "customer", Status: "active"}, nil)
	repo.On("UpdateChatBlocked", ctx, uint(5), true).Return(nil)
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	status, body := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/5/chat-block", `{"blocked":true,"reason":"spam link di chat"}`)

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, true, data["chat_blocked"])
	assert.Equal(t, "active", data["status"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_SetChatBlock_UnblocksWithReason(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(5)).Return(&entity.User{ID: 5, Email: "spam@example.com", Status: "active", ChatBlocked: true}, nil)
	repo.On("UpdateChatBlocked", ctx, uint(5), false).Return(nil)
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	status, body := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/5/chat-block", `{"blocked":false,"reason":"banding diterima"}`)

	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, false, body["data"].(map[string]interface{})["chat_blocked"])
	repo.AssertExpectations(t)
}

func TestAdminHandler_SetChatBlock_UserNotFound_404(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := new(mockUserRepo)
	repo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/99/chat-block", `{"blocked":true,"reason":"spam"}`)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "USER_NOT_FOUND", resp["code"])
}

func TestAdminHandler_SetChatBlock_InvalidID_400(t *testing.T) {
	t.Parallel()
	repo := new(mockUserRepo)
	app := newAdminChatTestApp(t, repo, &stubChatDirectory{})

	status, resp := doAdminUserRequest(t, app, "PATCH", "/api/v1/admin/users/abc/chat-block", `{"blocked":true,"reason":"spam"}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "INVALID_ID", resp["code"])
}
