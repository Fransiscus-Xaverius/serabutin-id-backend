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

		// Fixed window: the TTL is set only when the window is created.
		// Re-setting it on every request would extend the window under
		// continuous traffic (e.g. 30s polling), so the counter could
		// accumulate past the limit and never reset.
		pipe := rl.client.Pipeline()
		pipe.SetNX(ctx, key, 0, rl.window)
		incr := pipe.Incr(ctx, key)

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
