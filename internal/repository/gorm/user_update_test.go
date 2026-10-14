package gormrepo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
)

// Regression: Update must not clobber created_at. Previously Update built a
// UserModel without CreatedAt and called db.Save, zeroing created_at to
// 0001-01-01 on every write (e.g. PromoteAdmins on boot).

func TestUserRepository_Update_PreservesCreatedAt(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &entity.User{
		Email:    "update-ts@example.com",
		Password: "hashedpassword",
		Name:     "TS User",
		WhatsApp: "081234567893",
		Role:     "customer",
	}
	require.NoError(t, repo.Create(ctx, user))
	originalCreatedAt := user.CreatedAt
	require.False(t, originalCreatedAt.IsZero())
	originalUpdatedAt := user.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	found, err := repo.FindByEmail(ctx, "update-ts@example.com")
	require.NoError(t, err)
	found.Role = "admin"
	found.Name = "TS User Renamed"
	require.NoError(t, repo.Update(ctx, found))

	after, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", after.Role)
	assert.Equal(t, "TS User Renamed", after.Name)
	assert.WithinDuration(t, originalCreatedAt, after.CreatedAt, time.Millisecond,
		"created_at must survive Update: got %v want %v", after.CreatedAt, originalCreatedAt)
	assert.False(t, after.CreatedAt.IsZero())
	assert.False(t, after.UpdatedAt.Before(originalUpdatedAt),
		"updated_at must bump: got %v was %v", after.UpdatedAt, originalUpdatedAt)
}

// Worst case: caller passes an entity with zero CreatedAt (timestamps never
// loaded). Update must still preserve the stored created_at.
func TestUserRepository_Update_ZeroCreatedAtEntity_PreservesCreatedAt(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	seed := &entity.User{
		Email:    "update-zero-ts@example.com",
		Password: "hashedpassword",
		Name:     "Zero TS",
		WhatsApp: "081234567894",
		Role:     "customer",
	}
	require.NoError(t, repo.Create(ctx, seed))
	originalCreatedAt := seed.CreatedAt

	stale := &entity.User{
		ID:       seed.ID,
		Email:    seed.Email,
		Password: seed.Password,
		Name:     "Zero TS Renamed",
		WhatsApp: seed.WhatsApp,
		Role:     "admin",
		Status:   entity.UserStatusActive,
		// CreatedAt/UpdatedAt intentionally zero.
	}
	require.NoError(t, repo.Update(ctx, stale))

	after, err := repo.FindByID(ctx, seed.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", after.Role)
	assert.WithinDuration(t, originalCreatedAt, after.CreatedAt, time.Millisecond,
		"created_at must survive Update with zero timestamps: got %v want %v",
		after.CreatedAt, originalCreatedAt)
}

// Full boot path: PromoteAdmins -> Update must not zero created_at.
func TestPromoteAdmins_PreservesCreatedAt(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	seed := &entity.User{
		Email:    "promote-ts@example.com",
		Password: "hashedpassword",
		Name:     "Promote TS",
		WhatsApp: "081234567895",
		Role:     "customer",
	}
	require.NoError(t, repo.Create(ctx, seed))
	originalCreatedAt := seed.CreatedAt

	promoted := usecase.PromoteAdmins(ctx, repo, []string{"promote-ts@example.com"})
	require.Equal(t, []string{"promote-ts@example.com"}, promoted)

	after, err := repo.FindByID(ctx, seed.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", after.Role)
	assert.WithinDuration(t, originalCreatedAt, after.CreatedAt, time.Millisecond,
		"created_at must survive PromoteAdmins: got %v want %v",
		after.CreatedAt, originalCreatedAt)
}
