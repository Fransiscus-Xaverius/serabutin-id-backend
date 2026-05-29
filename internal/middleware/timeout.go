package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

func Timeout(timeout time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		done := make(chan error, 1)
		go func() {
			done <- c.Next()
		}()

		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			apiErr := apierror.NewAPIError(fiber.StatusRequestTimeout, "REQUEST_TIMEOUT", "request timed out")
			return apierror.GlobalErrorHandler(c, apiErr)
		}
	}
}
