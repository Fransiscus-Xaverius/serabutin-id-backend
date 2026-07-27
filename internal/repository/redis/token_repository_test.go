package redisrepo

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

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
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

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	_, err = client.Ping(ctx).Result()
	require.NoError(t, err)

	return client
}

func TestTokenRepository_StoreRefreshToken_Success(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	err := repo.StoreRefreshToken(context.Background(), 1, "test-uuid-123", 5*time.Minute)

	require.NoError(t, err)

	userID, err := repo.IsRefreshTokenValid(context.Background(), "test-uuid-123")
	require.NoError(t, err)
	assert.Equal(t, uint(1), userID)
}

func TestTokenRepository_IsRefreshTokenValid_ValidToken_ReturnsUserID(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	require.NoError(t, repo.StoreRefreshToken(context.Background(), 42, "valid-uuid", 5*time.Minute))

	userID, err := repo.IsRefreshTokenValid(context.Background(), "valid-uuid")

	require.NoError(t, err)
	assert.Equal(t, uint(42), userID)
}

func TestTokenRepository_IsRefreshTokenValid_ExpiredToken_ReturnsError(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	require.NoError(t, repo.StoreRefreshToken(context.Background(), 1, "expiring-uuid", 1*time.Second))

	time.Sleep(2 * time.Second)

	_, err := repo.IsRefreshTokenValid(context.Background(), "expiring-uuid")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrTokenRevoked, domainErr.Err)
}

func TestTokenRepository_RevokeRefreshToken_Success(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	require.NoError(t, repo.StoreRefreshToken(context.Background(), 1, "revoke-uuid", 5*time.Minute))

	err := repo.RevokeRefreshToken(context.Background(), "revoke-uuid")
	require.NoError(t, err)

	_, err = repo.IsRefreshTokenValid(context.Background(), "revoke-uuid")
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrTokenRevoked, domainErr.Err)
}

func TestTokenRepository_RevokeAllUserTokens_Success(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	require.NoError(t, repo.StoreRefreshToken(context.Background(), 1, "token-1", 5*time.Minute))
	require.NoError(t, repo.StoreRefreshToken(context.Background(), 1, "token-2", 5*time.Minute))
	require.NoError(t, repo.StoreRefreshToken(context.Background(), 2, "other-user-token", 5*time.Minute))

	err := repo.RevokeAllUserTokens(context.Background(), 1)
	require.NoError(t, err)

	_, err = repo.IsRefreshTokenValid(context.Background(), "token-1")
	require.Error(t, err)

	_, err = repo.IsRefreshTokenValid(context.Background(), "token-2")
	require.Error(t, err)

	otherUserID, err := repo.IsRefreshTokenValid(context.Background(), "other-user-token")
	require.NoError(t, err)
	assert.Equal(t, uint(2), otherUserID)
}

func TestTokenRepository_BlacklistAccessToken_Success(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	err := repo.BlacklistAccessToken(context.Background(), "access-uuid-123", 15*time.Minute)
	require.NoError(t, err)

	blacklisted, err := repo.IsAccessTokenBlacklisted(context.Background(), "access-uuid-123")
	require.NoError(t, err)
	assert.True(t, blacklisted)
}

func TestTokenRepository_IsAccessTokenBlacklisted_NotBlacklisted_ReturnsFalse(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	blacklisted, err := repo.IsAccessTokenBlacklisted(context.Background(), "clean-uuid")

	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestTokenRepository_StoreTokenPair_DoesNotBlacklistAccessToken(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	repo := NewTokenRepository(client)

	td := &entity.TokenDetails{
		AccessToken:  "some-access-token",
		RefreshToken: "some-refresh-token",
		AccessUUID:   "access-uuid",
		RefreshUUID:  "refresh-uuid",
		AtExpires:    time.Now().Add(15 * time.Minute),
		RtExpires:    time.Now().Add(7 * 24 * time.Hour),
	}

	err := repo.StoreTokenPair(context.Background(), td, 1)
	require.NoError(t, err)

	userID, err := repo.IsRefreshTokenValid(context.Background(), "refresh-uuid")
	require.NoError(t, err)
	assert.Equal(t, uint(1), userID)

	blacklisted, err := repo.IsAccessTokenBlacklisted(context.Background(), "access-uuid")
	require.NoError(t, err)
	assert.False(t, blacklisted)
}
