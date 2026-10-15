package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 12: kill-switch admin endpoints — list states, flip modules with
// mandatory reason, master fat-finger guard.

func errTestKillSwitchNotFound() error {
	return testKillSwitchNotFoundErr{}
}

type testKillSwitchNotFoundErr struct{}

func (testKillSwitchNotFoundErr) Error() string { return "kill switch not found" }

func errTestKillSwitchMiss() error {
	return testKillSwitchMissErr{}
}

type testKillSwitchMissErr struct{}

func (testKillSwitchMissErr) Error() string { return "killswitch: cache miss" }

type stubKillSwitchRepo struct {
	rows map[string]*entity.KillSwitch
}

func (s *stubKillSwitchRepo) List(ctx context.Context) ([]*entity.KillSwitch, error) {
	out := make([]*entity.KillSwitch, 0, len(s.rows))
	for _, m := range entity.KillSwitchModules {
		if sw, ok := s.rows[m]; ok {
			out = append(out, sw)
		}
	}
	return out, nil
}

func (s *stubKillSwitchRepo) Get(ctx context.Context, module string) (*entity.KillSwitch, error) {
	if sw, ok := s.rows[module]; ok {
		return sw, nil
	}
	return nil, errTestKillSwitchNotFound()
}

func (s *stubKillSwitchRepo) Upsert(ctx context.Context, sw *entity.KillSwitch) error {
	s.rows[sw.Module] = sw
	return nil
}

type stubKillSwitchCache struct {
	sets map[string]string
}

func (s *stubKillSwitchCache) Get(ctx context.Context, key string) (string, error) {
	return "", errTestKillSwitchMiss()
}

func (s *stubKillSwitchCache) Set(ctx context.Context, key, value string) error {
	s.sets[key] = value
	return nil
}

func newKillSwitchTestApp() (*fiber.App, *stubKillSwitchCache) {
	repo := &stubKillSwitchRepo{rows: map[string]*entity.KillSwitch{}}
	for _, m := range entity.KillSwitchModules {
		repo.rows[m] = &entity.KillSwitch{Module: m, Enabled: true, UpdatedBy: 7, Reason: "default seed"}
	}
	cache := &stubKillSwitchCache{sets: map[string]string{}}
	uc := usecase.NewKillSwitchUsecase(repo, cache)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetKillSwitchUC(uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/kill-switches", h.ListKillSwitches)
	app.Patch("/api/v1/admin/kill-switches/:module", h.SetKillSwitch)
	return app, cache
}

func doKillSwitchRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func TestAdminHandler_ListKillSwitches_ReturnsAllModules(t *testing.T) {
	t.Parallel()
	app, _ := newKillSwitchTestApp()

	status, body := doKillSwitchRequest(t, app, "GET", "/api/v1/admin/kill-switches", "")

	assert.Equal(t, fiber.StatusOK, status)
	rows := body["data"].(map[string]interface{})["data"].([]interface{})
	require.Len(t, rows, 5)
	modules := map[string]bool{}
	for _, r := range rows {
		row := r.(map[string]interface{})
		modules[row["module"].(string)] = row["enabled"].(bool)
		assert.Contains(t, row, "updated_by")
		assert.Contains(t, row, "updated_at")
		assert.Contains(t, row, "reason")
	}
	for _, m := range entity.KillSwitchModules {
		enabled, ok := modules[m]
		require.True(t, ok, "module %q missing", m)
		assert.True(t, enabled, "module %q should seed enabled", m)
	}
}

func TestAdminHandler_SetKillSwitch_MissingReason_400(t *testing.T) {
	t.Parallel()
	app, _ := newKillSwitchTestApp()

	for _, body := range []string{"", "{}", `{"enabled":false}`, `{"enabled":false,"reason":""}`, `{"enabled":false,"reason":"   "}`} {
		status, resp := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/bookings", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
		assert.Equal(t, "VALIDATION_ERROR", resp["code"], "body %q", body)
	}
}

func TestAdminHandler_SetKillSwitch_MissingEnabled_400(t *testing.T) {
	t.Parallel()
	app, _ := newKillSwitchTestApp()

	status, resp := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/bookings", `{"reason":"spam"}`)
	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", resp["code"])
}

func TestAdminHandler_SetKillSwitch_UnknownModule_404(t *testing.T) {
	t.Parallel()
	app, _ := newKillSwitchTestApp()

	status, body := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/payments", `{"enabled":false,"reason":"fraud"}`)
	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "KILL_SWITCH_NOT_FOUND", body["code"])
}

func TestAdminHandler_SetKillSwitch_MasterWithoutConfirm_400(t *testing.T) {
	t.Parallel()
	app, _ := newKillSwitchTestApp()

	for _, body := range []string{
		`{"enabled":false,"reason":"spam wave"}`,
		`{"enabled":false,"reason":"spam wave","confirm":"UNFREEZE"}`,
		`{"enabled":true,"reason":"calm","confirm":"FREEZE"}`,
	} {
		status, _ := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/master", body)
		assert.Equal(t, fiber.StatusBadRequest, status, "body %q", body)
	}
}

func TestAdminHandler_SetKillSwitch_DisableBookings_200AndPublishes(t *testing.T) {
	t.Parallel()
	app, cache := newKillSwitchTestApp()

	status, body := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/bookings", `{"enabled":false,"reason":"spam wave"}`)

	assert.Equal(t, fiber.StatusOK, status)
	row := body["data"].(map[string]interface{})
	assert.Equal(t, "bookings", row["module"])
	assert.Equal(t, false, row["enabled"])
	assert.Equal(t, float64(7), row["updated_by"])
	assert.Equal(t, "spam wave", row["reason"])
	assert.Equal(t, "0", cache.sets["killswitch:bookings"])
}

func TestAdminHandler_SetKillSwitch_MasterFreeze_200(t *testing.T) {
	t.Parallel()
	app, cache := newKillSwitchTestApp()

	status, body := doKillSwitchRequest(t, app, "PATCH", "/api/v1/admin/kill-switches/master", `{"enabled":false,"reason":"spam wave","confirm":"FREEZE"}`)

	assert.Equal(t, fiber.StatusOK, status)
	row := body["data"].(map[string]interface{})
	assert.Equal(t, "master", row["module"])
	assert.Equal(t, false, row["enabled"])
	assert.Equal(t, "0", cache.sets["killswitch:master"])
}
