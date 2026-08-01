package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

type RateLimiter struct {
	client       *redis.Client
	limit        int
	window       time.Duration
	keyGenerator func(fiber.Ctx) string
}

func NewRateLimiter(client *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client: client,
		limit:  limit,
		window: window,
		keyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
	}
}

func (rl *RateLimiter) Limit() fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := c.Context()
		ip := rl.keyGenerator(c)

		key := fmt.Sprintf("ratelimit:%s", ip)

		pipe := rl.client.Pipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, rl.window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			return c.Next()
		}

		count := incr.Val()
		if count > int64(rl.limit) {
			ttl, _ := rl.client.TTL(ctx, key).Result()
			retryAfter := int(ttl.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}

			apiErr := apierror.NewAPIError(
				fiber.StatusTooManyRequests,
				"RATE_LIMIT_EXCEEDED",
				fmt.Sprintf("too many requests, retry after %d seconds", retryAfter),
			)

			c.Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		return c.Next()
	}
}
