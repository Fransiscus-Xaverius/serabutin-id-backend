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

		// Fixed window: the TTL is armed when the key is created and never
		// extended afterwards. ExpireNX (set expiry only when none exists)
		// also heals keys left without a TTL by earlier versions or by the
		// race between creation and increment — otherwise the counter would
		// accumulate forever and lock the client out permanently.
		incr := rl.client.Incr(ctx, key)
		if incr.Err() != nil {
			return c.Next()
		}
		if err := rl.client.ExpireNX(ctx, key, rl.window).Err(); err != nil {
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
