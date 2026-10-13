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
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	retryAfter := resp.Header.Get("Retry-After")
	assert.NotEmpty(t, retryAfter)
}

func TestRateLimiter_Limit_WindowNotExtendedByLaterRequests(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 2, 1500*time.Millisecond)
	app := setupRateLimitTestApp(t, rl)

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	time.Sleep(400 * time.Millisecond)

	req = httptest.NewRequest("GET", "/test", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	time.Sleep(200 * time.Millisecond)

	// Rejected request: must not move the window end.
	req = httptest.NewRequest("GET", "/test", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	// Past the original window (t=1.5s since first request), a new window
	// must have started even though the last request happened at t=0.6s.
	time.Sleep(1000 * time.Millisecond)

	req = httptest.NewRequest("GET", "/test", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// Spec 010 follow-up: a key left WITHOUT a TTL (legacy writer, or the
// create/increment race) must be re-armed, otherwise the counter never
// resets and the client is locked out permanently.
func TestRateLimiter_Limit_ReArmsMissingTTL(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 3, time.Minute)
	app := setupRateLimitTestApp(t, rl)

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var key string
	keys, err := client.Keys(context.Background(), "ratelimit:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	key = keys[0]

	// Simulate the raced/legacy state: counter above the limit, no expiry.
	require.NoError(t, client.Set(context.Background(), key, 999, 0).Err())

	req = httptest.NewRequest("GET", "/test", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	ttl, err := client.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "TTL must be re-armed, not -1")
	assert.LessOrEqual(t, ttl, time.Minute)
}

func TestRateLimiter_Limit_DifferentIP_NotAffected(t *testing.T) {
	t.Parallel()

	client := setupTestRedisForMiddleware(t)
	rl := NewRateLimiter(client, 3, 1*time.Minute)
	rl.keyGenerator = func(c fiber.Ctx) string { return c.Get("X-Test-IP") }
	app := setupRateLimitTestApp(t, rl)

	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Test-IP", "172.16.0.1")
		_, err := app.Test(req)
		require.NoError(t, err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Test-IP", "172.16.0.2")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
