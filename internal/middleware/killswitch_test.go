package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ticket 12: RequireModule — 503 MODULE_DISABLED while a module (or master)
// is off, pass-through otherwise, fail-open on checker outage.

type stubKillSwitchChecker struct {
	enabled map[string]bool
	errMode bool // when true, report every module enabled (outage fail-open)
}

func (s *stubKillSwitchChecker) IsEnabled(ctx context.Context, module string) bool {
	if s.errMode {
		return true
	}
	return s.enabled[module]
}

func killSwitchGateApp(checker KillSwitchChecker, module string) *fiber.App {
	app := fiber.New()
	app.Post("/bookings", RequireModule(checker, module), func(c fiber.Ctx) error {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})
	return app
}

func doGateRequest(t *testing.T, app *fiber.App) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("POST", "/bookings", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func TestRequireModule_Enabled_PassesThrough(t *testing.T) {
	t.Parallel()
	app := killSwitchGateApp(&stubKillSwitchChecker{enabled: map[string]bool{"bookings": true}}, "bookings")

	status, _ := doGateRequest(t, app)
	assert.Equal(t, fiber.StatusCreated, status)
}

func TestRequireModule_Disabled_503WithModule(t *testing.T) {
	t.Parallel()
	app := killSwitchGateApp(&stubKillSwitchChecker{enabled: map[string]bool{"bookings": false}}, "bookings")

	status, body := doGateRequest(t, app)
	assert.Equal(t, fiber.StatusServiceUnavailable, status)
	assert.Equal(t, "MODULE_DISABLED", body["code"])
	assert.Contains(t, body["message"], "bookings")
}

func TestRequireModule_NilChecker_FailsOpen(t *testing.T) {
	t.Parallel()
	app := killSwitchGateApp(nil, "bookings")

	status, _ := doGateRequest(t, app)
	assert.Equal(t, fiber.StatusCreated, status)
}

func TestRequireModule_CheckerOutage_FailsOpen(t *testing.T) {
	t.Parallel()
	// A Redis+DB outage must not freeze writes: the checker answers
	// enabled when it cannot reach state.
	app := killSwitchGateApp(&stubKillSwitchChecker{errMode: true}, "bookings")

	status, _ := doGateRequest(t, app)
	assert.Equal(t, fiber.StatusCreated, status)
}
