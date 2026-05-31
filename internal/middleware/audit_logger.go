package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/serabutin/serabutin-id-backend/pkg/redact"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func AuditLogger(log zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start).Milliseconds()

		path := c.Path()
		if !strings.HasPrefix(path, "/api/v1/auth/") {
			return err
		}

		statusCode := c.Response().StatusCode()
		event := determineAuthEvent(path, c.Method(), err, statusCode)
		if event == "" {
			return err
		}

		level := zerolog.InfoLevel
		if err != nil || strings.HasSuffix(event, ".failure") || strings.HasSuffix(event, ".triggered") {
			level = zerolog.WarnLevel
		}

		evt := log.WithLevel(level).
			Str("event", event).
			Str("request_id", requestctx.GetRequestID(c)).
			Str("ip", c.IP()).
			Str("user_agent", c.Get("User-Agent")).
			Int64("duration_ms", duration)

		userID := requestctx.GetUserID(c)
		if userID > 0 {
			evt.Uint("user_id", userID)
		}

		evt.Msg("auth event")

		return err
	}
}

func determineAuthEvent(path, method string, handlerErr error, statusCode int) string {
	isFailure := handlerErr != nil || statusCode >= 400
	switch {
	case strings.HasSuffix(path, "/auth/register") && method == "POST":
		if isFailure {
			return "auth.register.failure"
		}
		return "auth.register.success"
	case strings.HasSuffix(path, "/auth/login") && method == "POST":
		if isFailure {
			return "auth.login.failure"
		}
		return "auth.login.success"
	case strings.HasSuffix(path, "/auth/logout") && method == "POST":
		return "auth.logout"
	case strings.HasSuffix(path, "/auth/refresh") && method == "POST":
		if isFailure {
			return "auth.refresh.failure"
		}
		return "auth.refresh.success"
	}
	return ""
}

func RedactIdentifier(identifier string) string {
	if strings.Contains(identifier, "@") {
		return redact.RedactEmail(identifier)
	}
	return redact.RedactPhone(identifier)
}
