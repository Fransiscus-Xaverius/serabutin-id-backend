package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	fibertimeout "github.com/gofiber/fiber/v3/middleware/timeout"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

func Timeout(timeout time.Duration) fiber.Handler {
	return fibertimeout.New(func(c fiber.Ctx) error {
		return c.Next()
	}, fibertimeout.Config{
		Timeout: timeout,
		OnTimeout: func(c fiber.Ctx) error {
			apiErr := apierror.NewAPIError(fiber.StatusRequestTimeout, "REQUEST_TIMEOUT", "request timed out")
			return apierror.GlobalErrorHandler(c, apiErr)
		},
	})
}
