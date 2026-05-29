package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type AuthMiddleware struct {
	jwtSecret     string
	tokenRepo     repository.TokenRepository
}

func NewAuthMiddleware(jwtSecret string, tokenRepo repository.TokenRepository) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSecret: jwtSecret,
		tokenRepo: tokenRepo,
	}
}

func (m *AuthMiddleware) Authenticate() fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "MISSING_TOKEN", "authorization header is required")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_HEADER", "authorization header must be Bearer {token}")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apperror.ErrTokenInvalid
			}
			return []byte(m.jwtSecret), nil
		})
		if err != nil || !token.Valid {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_TOKEN", "token is invalid or expired")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_TOKEN", "invalid token claims")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		accessUUID, ok := claims["access_uuid"].(string)
		if !ok {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_TOKEN", "invalid token claims")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		if m.tokenRepo != nil {
			blacklisted, err := m.tokenRepo.IsAccessTokenBlacklisted(c.Context(), accessUUID)
			if err != nil {
				apiErr := apierror.FromDomainError(err)
				return apierror.GlobalErrorHandler(c, apiErr)
			}
			if blacklisted {
				apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "TOKEN_REVOKED", "token has been revoked")
				return apierror.GlobalErrorHandler(c, apiErr)
			}
		}

		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_TOKEN", "invalid token claims")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		email, _ := claims["email"].(string)

		requestctx.SetUserID(c, uint(userIDFloat))
		requestctx.SetUserEmail(c, email)

		return c.Next()
	}
}
