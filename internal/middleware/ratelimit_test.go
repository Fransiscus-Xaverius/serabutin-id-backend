package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

func setupTestRedisForMiddleware(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	redisContainer, err := rediscontainer.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithOccurrence(1).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	_, err = client.Ping(ctx).Result()
	require.NoError(t, err)

	return client
}

func setupRateLimitTestApp(t *testing.T, rl *RateLimiter) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: apierror.GlobalErrorHandler,
	})

	app.Use(rl.Limit())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	return app
}

func TestRateLimiter_Limit_UnderLimit_Passes(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 3, 1*time.Minute)
	app := setupRateLimitTestApp(t, rl)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"

		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}
}

func TestRateLimiter_Limit_OverLimit_Returns429(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 3, 1*time.Minute)
	app := setupRateLimitTestApp(t, rl)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	retryAfter := resp.Header.Get("Retry-After")
	assert.NotEmpty(t, retryAfter)
}

func TestRateLimiter_Limit_DifferentIP_NotAffected(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 3, 1*time.Minute)
	app := setupRateLimitTestApp(t, rl)

	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "172.16.0.1:12345"
		_, err := app.Test(req)
		require.NoError(t, err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "172.16.0.2:12345"

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
