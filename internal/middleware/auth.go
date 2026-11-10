package middleware

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type AuthMiddleware struct {
	jwtSecret string
	tokenRepo repository.TokenRepository
	userRepo  repository.UserRepository
}

func NewAuthMiddleware(jwtSecret string, tokenRepo repository.TokenRepository) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSecret: jwtSecret,
		tokenRepo: tokenRepo,
	}
}

// SetUserRepository attaches the user directory for the suspended-account
// gate (F1). Kept as a setter so the established constructor shape — relied
// on by existing middleware tests — stays stable. Nil means ungated.
func (m *AuthMiddleware) SetUserRepository(repo repository.UserRepository) {
	m.userRepo = repo
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

		// F1: suspended users keep no live sessions. Even a cryptographically
		// valid, non-revoked token is rejected once the account is suspended
		// (401 ACCOUNT_SUSPENDED). Lookup outages fail open (logged) so a
		// user-directory blip never locks out every active user; the Suspend
		// path additionally revokes tokens, so revocation is the hard kill
		// and this gate is the backstop.
		if m.userRepo != nil {
			if user, err := m.userRepo.FindByID(c.Context(), uint(userIDFloat)); err != nil {
				log.Printf("[AuthMiddleware.Authenticate] status lookup failed for user %d: %v", uint(userIDFloat), err)
			} else if user != nil && user.Status == entity.UserStatusSuspended {
				apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "ACCOUNT_SUSPENDED", "account is suspended")
				return apierror.GlobalErrorHandler(c, apiErr)
			}
		}

		return c.Next()
	}
}
