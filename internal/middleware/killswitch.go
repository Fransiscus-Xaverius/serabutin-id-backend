package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

// KillSwitchChecker is the read surface RequireModule needs. Both
// *usecase.KillSwitchUsecase and test fakes implement it. Defined here so
// middleware never imports the usecase package.
type KillSwitchChecker interface {
	IsEnabled(ctx context.Context, module string) bool
}

// RequireModule blocks user-facing writes while a kill-switch module (or
// the master freeze) is off: 503 MODULE_DISABLED with the module in the
// body. Reads fail OPEN — when the checker cannot answer (Redis+DB down)
// IsEnabled returns true, so an infra outage never freezes the platform.
// Admin reads are unaffected (this middleware is only mounted on write
// routes). A nil checker disables the gate (fail-open by construction).
func RequireModule(checker KillSwitchChecker, module string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if checker == nil || checker.IsEnabled(c.Context(), module) {
			return c.Next()
		}
		err := apierror.NewAPIError(fiber.StatusServiceUnavailable, "MODULE_DISABLED", "module '"+module+"' is temporarily disabled")
		err.Details = map[string]string{"module": module}
		return apierror.GlobalErrorHandler(c, err)
	}
}
