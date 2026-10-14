package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

type stubAuditLogRepo struct {
	items []*entity.AuditLog
	last  repository.AuditLogFilter
}

func (s *stubAuditLogRepo) Create(_ context.Context, _ *entity.AuditLog) error {
	return nil
}

func (s *stubAuditLogRepo) List(_ context.Context, f repository.AuditLogFilter) ([]*entity.AuditLog, int64, error) {
	s.last = f
	out := make([]*entity.AuditLog, 0, len(s.items))
	for _, it := range s.items {
		if f.ActorUserID != 0 && it.ActorUserID != f.ActorUserID {
			continue
		}
		if f.Action != "" && it.Action != f.Action {
			continue
		}
		out = append(out, it)
	}
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	start := (page - 1) * limit
	if start >= len(out) {
		return []*entity.AuditLog{}, int64(len(out)), nil
	}
	end := start + limit
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], int64(len(out)), nil
}

func auditLogsTestApp(h *AdminHandler) *fiber.App {
	app := fiber.New()
	app.Get("/admin/audit-logs", h.ListAuditLogs)
	return app
}

func TestAdminHandler_ListAuditLogs_Paginated(t *testing.T) {
	t.Parallel()
	stub := &stubAuditLogRepo{items: []*entity.AuditLog{
		{ID: 1, ActorUserID: 7, Action: "a", TargetType: "user", TargetID: "1", Reason: "r1"},
		{ID: 2, ActorUserID: 8, Action: "b", TargetType: "booking", TargetID: "9", Reason: "r2"},
	}}
	h := NewAdminHandler(nil, nil, nil, nil, nil, stub, nil, nil, nil, nil)
	app := auditLogsTestApp(h)

	req := httptest.NewRequest("GET", "/admin/audit-logs?page=1&limit=20", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Data []struct {
				ID          uint   `json:"id"`
				ActorUserID uint   `json:"actor_user_id"`
				Action      string `json:"action"`
				TargetType  string `json:"target_type"`
				TargetID    string `json:"target_id"`
				Reason      string `json:"reason"`
			} `json:"data"`
			Pagination struct {
				Page       int   `json:"page"`
				Limit      int   `json:"limit"`
				Total      int64 `json:"total"`
				TotalPages int64 `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.Success)
	assert.Len(t, body.Data.Data, 2)
	assert.Equal(t, int64(2), body.Data.Pagination.Total)
	assert.Equal(t, "r1", body.Data.Data[0].Reason)
}

func TestAdminHandler_ListAuditLogs_FiltersByActorAndAction(t *testing.T) {
	t.Parallel()
	stub := &stubAuditLogRepo{items: []*entity.AuditLog{
		{ID: 1, ActorUserID: 7, Action: "user.suspend", Reason: "spam"},
		{ID: 2, ActorUserID: 8, Action: "user.suspend", Reason: "abuse"},
	}}
	h := NewAdminHandler(nil, nil, nil, nil, nil, stub, nil, nil, nil, nil)
	app := auditLogsTestApp(h)

	req := httptest.NewRequest("GET", "/admin/audit-logs?actor_id=7&action=user.suspend", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, uint(7), stub.last.ActorUserID)
	assert.Equal(t, "user.suspend", stub.last.Action)
}
