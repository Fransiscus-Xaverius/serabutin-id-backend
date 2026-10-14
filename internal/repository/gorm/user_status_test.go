package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// Ticket 1: status flag persists through the repository; empty defaults to active.

func TestUserRepository_Status_DefaultsToActive(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "status-default@example.com",
		Password: "hashedpassword",
		Name:     "Status Default",
		WhatsApp: "081234567891",
	}

	require.NoError(t, repo.Create(context.Background(), user))
	assert.Equal(t, entity.UserStatusActive, user.Status)

	found, err := repo.FindByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusActive, found.Status)
}

func TestUserRepository_Status_SuspendedRoundTrips(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "status-suspended@example.com",
		Password: "hashedpassword",
		Name:     "Status Suspended",
		WhatsApp: "081234567892",
		Status:   entity.UserStatusSuspended,
	}

	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByEmail(context.Background(), "status-suspended@example.com")
	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, found.Status)

	foundWA, err := repo.FindByWhatsApp(context.Background(), "081234567892")
	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, foundWA.Status)
}
