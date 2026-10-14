package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Ticket 3: filterable admin user list + column-scoped status/role updates.

func seedAdminUserList(t *testing.T, repo *UserRepository) {
	t.Helper()
	ctx := context.Background()
	seed := []*entity.User{
		{Email: "budi@example.com", Password: "h", Name: "Budi Santoso", WhatsApp: "081111111111", Role: "customer", Status: entity.UserStatusActive},
		{Email: "mitra@example.com", Password: "h", Name: "Mitra Bersih", WhatsApp: "082222222222", Role: "provider", Status: entity.UserStatusActive},
		{Email: "banned@example.com", Password: "h", Name: "Banned Budi", WhatsApp: "083333333333", Role: "customer", Status: entity.UserStatusSuspended},
		{Email: "boss@example.com", Password: "h", Name: "Boss Admin", WhatsApp: "084444444444", Role: "admin", Status: entity.UserStatusActive},
	}
	for _, u := range seed {
		require.NoError(t, repo.Create(ctx, u))
	}
}

func TestUserRepository_List_SearchMatchesEmailNameWhatsApp(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	seedAdminUserList(t, repo)
	ctx := context.Background()

	rows, total, err := repo.List(ctx, repository.UserFilter{Q: "budi", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	// Newest first.
	assert.Equal(t, "banned@example.com", rows[0].Email)
	assert.Equal(t, "budi@example.com", rows[1].Email)

	rows, total, err = repo.List(ctx, repository.UserFilter{Q: "082222222222", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "mitra@example.com", rows[0].Email)
}

func TestUserRepository_List_FiltersRoleAndStatus(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	seedAdminUserList(t, repo)
	ctx := context.Background()

	rows, total, err := repo.List(ctx, repository.UserFilter{Role: "customer", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, rows, 2)

	rows, total, err = repo.List(ctx, repository.UserFilter{Status: entity.UserStatusSuspended, Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "banned@example.com", rows[0].Email)

	rows, total, err = repo.List(ctx, repository.UserFilter{Role: "customer", Status: entity.UserStatusActive, Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "budi@example.com", rows[0].Email)
}

func TestUserRepository_List_PaginatesWithTotal(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	seedAdminUserList(t, repo)
	ctx := context.Background()

	rows, total, err := repo.List(ctx, repository.UserFilter{Page: 2, Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, rows, 1)

	rows, total, err = repo.List(ctx, repository.UserFilter{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	require.Len(t, rows, 2)
	assert.Greater(t, rows[0].ID, rows[1].ID)
}

func TestUserRepository_UpdateStatus_And_UpdateRole(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	seedAdminUserList(t, repo)
	ctx := context.Background()

	rows, _, err := repo.List(ctx, repository.UserFilter{Email: "budi@example.com", Page: 1, Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID

	require.NoError(t, repo.UpdateStatus(ctx, id, entity.UserStatusSuspended))
	found, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, entity.UserStatusSuspended, found.Status)
	assert.Equal(t, "customer", found.Role)

	require.NoError(t, repo.UpdateRole(ctx, id, "provider"))
	found, err = repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "provider", found.Role)
	assert.Equal(t, entity.UserStatusSuspended, found.Status)
}

func TestUserRepository_UpdateStatus_MissingUser_NotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	err := repo.UpdateStatus(context.Background(), 999999, entity.UserStatusSuspended)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}
