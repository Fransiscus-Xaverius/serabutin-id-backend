package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogger_LogsAuthSuccess(t *testing.T) {
	t.Parallel()
	var logOutput strings.Builder
	logger := zerolog.New(&logOutput).With().Timestamp().Logger()

	app := fiber.New()
	app.Use(AuditLogger(logger))
	app.Post("/api/v1/auth/login", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	log := logOutput.String()
	assert.Contains(t, log, "auth.login.success")
	assert.Contains(t, log, "test-agent")
}

func TestAuditLogger_LogsAuthFailure(t *testing.T) {
	t.Parallel()
	var logOutput strings.Builder
	logger := zerolog.New(&logOutput).With().Timestamp().Logger()

	app := fiber.New()
	app.Use(AuditLogger(logger))
	app.Post("/api/v1/auth/login", func(c fiber.Ctx) error {
		return c.SendStatus(401)
	})

	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	log := logOutput.String()
	assert.Contains(t, log, "auth.login.failure")
}

func TestAuditLogger_SkipsNonAuthPaths(t *testing.T) {
	t.Parallel()
	var logOutput strings.Builder
	logger := zerolog.New(&logOutput).With().Timestamp().Logger()

	app := fiber.New()
	app.Use(AuditLogger(logger))
	app.Get("/health", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/health", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	log := logOutput.String()
	assert.Empty(t, log)
}

func TestRedactIdentifier_Email(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "t***@example.com", RedactIdentifier("test@example.com"))
}

func TestRedactIdentifier_Phone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "08******7890", RedactIdentifier("081234567890"))
}
