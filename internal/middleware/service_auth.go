package middleware

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

// RequireServiceToken guards service-to-service internal endpoints (e.g.
// GET /api/v1/internal/chat-access/:id). Only the shared service secret via
// X-Service-Token is accepted — user JWTs are never accepted here. An empty
// expected secret denies all (fail-closed): unlike user-facing introspect,
// these endpoints have no availability-first reason to fail open.
func RequireServiceToken(expected string) fiber.Handler {
	return func(c fiber.Ctx) error {
		got := c.Get("X-Service-Token")
		if expected == "" || got == "" {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "MISSING_SERVICE_TOKEN", "service token required"))
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_SERVICE_TOKEN", "invalid service token"))
		}
		return c.Next()
	}
}
