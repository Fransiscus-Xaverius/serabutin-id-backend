package service

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestRedis(t *testing.T) *redis.Client {
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

	client := redis.NewClient(&redis.Options{Addr: addr})
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err)
	return client
}

func TestLoginLimiter_IsLocked_InitialNotLocked(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 5, 15*time.Minute)

	locked, err := limiter.IsLocked(context.Background(), "test@example.com")
	require.NoError(t, err)
	assert.False(t, locked)
}

func TestLoginLimiter_RecordFailure_LocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 3, 15*time.Minute)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	}

	locked, err := limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.True(t, locked)
}

func TestLoginLimiter_RemainingAttempts(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 5, 15*time.Minute)

	ctx := context.Background()
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))

	remaining, err := limiter.RemainingAttempts(ctx, "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, 3, remaining)
}

func TestLoginLimiter_Reset_ClearsLockout(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 2, 15*time.Minute)

	ctx := context.Background()
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))

	locked, err := limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.True(t, locked)

	require.NoError(t, limiter.Reset(ctx, "test@example.com"))

	locked, err = limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.False(t, locked)
}
