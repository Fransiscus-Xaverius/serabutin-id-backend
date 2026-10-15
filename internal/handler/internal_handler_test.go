package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// F3: the service-to-service chat-access signal reports suspended and
// chat_blocked per user.

type mockInternalUserRepo struct {
	mock.Mock
}

func (m *mockInternalUserRepo) Create(ctx context.Context, user *entity.User) error {
	return m.Called(ctx, user).Error(0)
}
func (m *mockInternalUserRepo) Update(ctx context.Context, user *entity.User) error {
	return m.Called(ctx, user).Error(0)
}
func (m *mockInternalUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockInternalUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockInternalUserRepo) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	args := m.Called(ctx, whatsapp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}
func (m *mockInternalUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}
func (m *mockInternalUserRepo) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.User), args.Get(1).(int64), args.Error(2)
}
func (m *mockInternalUserRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return m.Called(ctx, id, status).Error(0)
}
func (m *mockInternalUserRepo) UpdateRole(ctx context.Context, id uint, role string) error {
	return m.Called(ctx, id, role).Error(0)
}
func (m *mockInternalUserRepo) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	return m.Called(ctx, id, blocked).Error(0)
}
func (m *mockInternalUserRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	return 2, nil
}

func TestInternalHandler_ChatAccess_ReportsSignals(t *testing.T) {
	t.Parallel()
	repo := new(mockInternalUserRepo)
	h := NewInternalHandler(repo)

	app := fiber.New()
	app.Get("/api/v1/internal/chat-access/:id", h.ChatAccess)

	repo.On("FindByID", mock.Anything, uint(9)).Return(&entity.User{
		ID: 9, Status: entity.UserStatusSuspended, ChatBlocked: true,
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/internal/chat-access/9", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			UserID      uint `json:"user_id"`
			Suspended   bool `json:"suspended"`
			ChatBlocked bool `json:"chat_blocked"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.Success)
	assert.Equal(t, uint(9), body.Data.UserID)
	assert.True(t, body.Data.Suspended)
	assert.True(t, body.Data.ChatBlocked)
	repo.AssertExpectations(t)
}

func TestInternalHandler_ChatAccess_UnknownUser_404(t *testing.T) {
	t.Parallel()
	repo := new(mockInternalUserRepo)
	h := NewInternalHandler(repo)

	app := fiber.New()
	app.Get("/api/v1/internal/chat-access/:id", h.ChatAccess)

	repo.On("FindByID", mock.Anything, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found"))

	req := httptest.NewRequest("GET", "/api/v1/internal/chat-access/99", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}
