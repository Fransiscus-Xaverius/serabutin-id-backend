package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

const loginAttemptsPrefix = "login_attempts:"

type LoginLimiter struct {
	client          *redis.Client
	maxAttempts     int
	lockoutDuration time.Duration
}

func NewLoginLimiter(client *redis.Client, maxAttempts int, lockoutDuration time.Duration) *LoginLimiter {
	return &LoginLimiter{
		client:          client,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
}

func (l *LoginLimiter) IsLocked(ctx context.Context, identifier string) (bool, error) {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	count, err := l.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return count >= int64(l.maxAttempts), nil
}

func (l *LoginLimiter) RecordFailure(ctx context.Context, identifier string) error {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	pipe := l.client.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, l.lockoutDuration)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return nil
}

func (l *LoginLimiter) Reset(ctx context.Context, identifier string) error {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	if err := l.client.Del(ctx, key).Err(); err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return nil
}

func (l *LoginLimiter) RemainingAttempts(ctx context.Context, identifier string) (int, error) {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	count, err := l.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return l.maxAttempts, nil
	}
	if err != nil {
		return 0, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	remaining := l.maxAttempts - int(count)
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}
