package redisrepo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

const (
	refreshTokenPrefix = "refresh_token:"
	accessBlacklistPrefix = "access_blacklist:"
	userTokensPrefix   = "user_tokens:"
)

type TokenRepository struct {
	client *redis.Client
}

func NewTokenRepository(client *redis.Client) *TokenRepository {
	return &TokenRepository{client: client}
}

func (r *TokenRepository) StoreRefreshToken(ctx context.Context, userID uint, tokenUUID string, expires time.Duration) error {
	key := fmt.Sprintf("%s%s", refreshTokenPrefix, tokenUUID)
	userKey := fmt.Sprintf("%s%d", userTokensPrefix, userID)

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, userID, expires)
	pipe.SAdd(ctx, userKey, tokenUUID)
	pipe.Expire(ctx, userKey, expires)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return nil
}

func (r *TokenRepository) IsRefreshTokenValid(ctx context.Context, tokenUUID string) (uint, error) {
	key := fmt.Sprintf("%s%s", refreshTokenPrefix, tokenUUID)

	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, apperror.NewDomainError(apperror.ErrTokenRevoked, "TOKEN_REVOKED", "refresh token has been revoked or expired")
	}
	if err != nil {
		return 0, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	userID, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, apperror.NewDomainError(apperror.ErrTokenInvalid, "TOKEN_INVALID", "invalid token data")
	}

	return uint(userID), nil
}

func (r *TokenRepository) RevokeRefreshToken(ctx context.Context, tokenUUID string) error {
	key := fmt.Sprintf("%s%s", refreshTokenPrefix, tokenUUID)

	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	userKey := fmt.Sprintf("%s%s", userTokensPrefix, val)

	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, userKey, tokenUUID)
	_, err = pipe.Exec(ctx)

	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return nil
}

func (r *TokenRepository) RevokeAllUserTokens(ctx context.Context, userID uint) error {
	userKey := fmt.Sprintf("%s%d", userTokensPrefix, userID)

	tokens, err := r.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	if len(tokens) == 0 {
		return nil
	}

	keys := make([]string, len(tokens))
	for i, token := range tokens {
		keys[i] = fmt.Sprintf("%s%s", refreshTokenPrefix, token)
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, keys...)
	pipe.Del(ctx, userKey)
	_, err = pipe.Exec(ctx)

	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return nil
}

func (r *TokenRepository) BlacklistAccessToken(ctx context.Context, tokenUUID string, expires time.Duration) error {
	key := fmt.Sprintf("%s%s", accessBlacklistPrefix, tokenUUID)

	if err := r.client.Set(ctx, key, "revoked", expires).Err(); err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return nil
}

func (r *TokenRepository) IsAccessTokenBlacklisted(ctx context.Context, tokenUUID string) (bool, error) {
	key := fmt.Sprintf("%s%s", accessBlacklistPrefix, tokenUUID)

	_, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return true, nil
}

func (r *TokenRepository) StoreTokenPair(ctx context.Context, td *entity.TokenDetails, userID uint) error {
	pipe := r.client.Pipeline()

	refreshKey := fmt.Sprintf("%s%s", refreshTokenPrefix, td.RefreshUUID)
	userKey := fmt.Sprintf("%s%d", userTokensPrefix, userID)
	rtTTL := time.Until(td.RtExpires)

	pipe.Set(ctx, refreshKey, userID, rtTTL)
	pipe.SAdd(ctx, userKey, td.RefreshUUID)
	pipe.Expire(ctx, userKey, rtTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}

	return nil
}
