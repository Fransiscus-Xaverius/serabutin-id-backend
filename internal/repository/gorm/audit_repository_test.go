package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func setupAuditTestDB(t *testing.T) *AuditLogRepository {
	t.Helper()
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AuditLogModel{}))
	return NewAuditLogRepository(db)
}

func TestAuditLogRepository_Create_PersistsRow(t *testing.T) {
	t.Parallel()
	repo := setupAuditTestDB(t)

	entry := &entity.AuditLog{
		ActorUserID: 7,
		Action:      "PATCH /api/v1/admin/users/:id/suspend",
		TargetType:  "user",
		TargetID:    "42",
		Reason:      "spam reports",
	}
	require.NoError(t, repo.Create(context.Background(), entry))
	assert.NotZero(t, entry.ID)
	assert.False(t, entry.CreatedAt.IsZero())
}

func TestAuditLogRepository_List_FiltersByActorAndAction(t *testing.T) {
	t.Parallel()
	repo := setupAuditTestDB(t)
	ctx := context.Background()

	seed := []*entity.AuditLog{
		{ActorUserID: 7, Action: "PATCH /api/v1/admin/users/:id/suspend", TargetType: "user", TargetID: "1", Reason: "spam"},
		{ActorUserID: 7, Action: "PATCH /api/v1/admin/bookings/:id/cancel", TargetType: "booking", TargetID: "9", Reason: "fraud"},
		{ActorUserID: 8, Action: "PATCH /api/v1/admin/users/:id/suspend", TargetType: "user", TargetID: "2", Reason: "abuse"},
	}
	for _, e := range seed {
		require.NoError(t, repo.Create(ctx, e))
	}

	items, total, err := repo.List(ctx, repository.AuditLogFilter{Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, items, 3)
	// Stable ordering: newest first.
	assert.Greater(t, items[0].ID, items[1].ID)

	items, total, err = repo.List(ctx, repository.AuditLogFilter{ActorUserID: 7, Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)
	for _, it := range items {
		assert.Equal(t, uint(7), it.ActorUserID)
	}

	items, total, err = repo.List(ctx, repository.AuditLogFilter{Action: "PATCH /api/v1/admin/users/:id/suspend", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)
}
