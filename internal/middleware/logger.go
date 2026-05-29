package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func RequestLogger(log zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		elapsed := time.Since(start)

		event := log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("latency", elapsed).
			Str("ip", c.IP()).
			Str("request_id", requestctx.GetRequestID(c))

		userID := requestctx.GetUserID(c)
		if userID != 0 {
			event.Uint("user_id", uint(userID))
		}

		if err != nil {
			event.Err(err).Msg("request completed with error")
		} else {
			event.Msg("request completed")
		}

		return err
	}
}
