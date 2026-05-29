package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

func setupTimeoutTestApp(t *testing.T, timeout time.Duration, handler fiber.Handler) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: apierror.GlobalErrorHandler,
	})

	app.Use(Timeout(timeout))
	app.Get("/test", handler)

	return app
}

func TestTimeout_WithinLimit_Passes(t *testing.T) {
	t.Parallel()

	fastHandler := func(c fiber.Ctx) error {
		return c.SendStatus(200)
	}

	app := setupTimeoutTestApp(t, 100*time.Millisecond, fastHandler)

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestTimeout_ExceedsLimit_Returns408(t *testing.T) {
	t.Parallel()

	slowHandler := func(c fiber.Ctx) error {
		time.Sleep(200 * time.Millisecond)
		return c.SendStatus(200)
	}

	app := setupTimeoutTestApp(t, 100*time.Millisecond, slowHandler)

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 408, resp.StatusCode)
}
