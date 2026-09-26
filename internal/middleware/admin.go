package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// profileLoader is the minimal surface RequireAdmin needs — implemented by
// *usecase.ProfileUsecase. Kept as an interface so middleware does not
// import the usecase package.
type profileLoader interface {
	GetProfile(ctx context.Context, userID uint) (*entity.User, error)
}

func RequireAdmin(loader profileLoader) fiber.Handler {
	return func(c fiber.Ctx) error {
		userID := requestctx.GetUserID(c)
		if userID == 0 {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
		}
		user, err := loader.GetProfile(c.Context(), userID)
		if err != nil || user == nil || user.Role != "admin" {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusForbidden, "FORBIDDEN", "admin access required"))
		}
		return c.Next()
	}
}
