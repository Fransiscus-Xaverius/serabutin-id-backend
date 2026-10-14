package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// Ticket 8: chat_blocked flag persists through the repository and survives
// full-row Update (profile edits must never clobber it).

func TestUserRepository_ChatBlocked_RoundTrips(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:       "chat-block@example.com",
		Password:    "hashedpassword",
		Name:        "Chat Block",
		WhatsApp:    "081234567893",
		ChatBlocked: true,
	}

	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.True(t, found.ChatBlocked)

	require.NoError(t, repo.UpdateChatBlocked(context.Background(), user.ID, false))
	found, err = repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.False(t, found.ChatBlocked)
}

func TestUserRepository_ChatBlocked_DefaultsToFalse(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "chat-block-default@example.com",
		Password: "hashedpassword",
		Name:     "Chat Default",
		WhatsApp: "081234567894",
	}

	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.False(t, found.ChatBlocked)
}

func TestUserRepository_Update_PreservesChatBlocked(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:       "chat-block-update@example.com",
		Password:    "hashedpassword",
		Name:        "Chat Update",
		WhatsApp:    "081234567895",
		ChatBlocked: true,
	}
	require.NoError(t, repo.Create(context.Background(), user))

	// Profile edits follow load-mutate-save: reload the row, change the
	// name, save — the chat_blocked column must survive the full-row Save.
	loaded, err := repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	loaded.Name = "Chat Update Renamed"
	require.NoError(t, repo.Update(context.Background(), loaded))

	found, err := repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chat Update Renamed", found.Name)
	assert.True(t, found.ChatBlocked)
}
