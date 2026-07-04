package handler

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
	"github.com/serabutin/serabutin-id-backend/pkg/whatsapp"
)

type AuthHandler struct {
	authUsecase *usecase.AuthUsecase
}

func NewAuthHandler(authUsecase *usecase.AuthUsecase) *AuthHandler {
	return &AuthHandler{
		authUsecase: authUsecase,
	}
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	normalizedWA := whatsapp.Normalize(req.WhatsApp)

	td, user, err := h.authUsecase.Register(c.Context(), req.Email, req.Password, req.Name, normalizedWA, req.Role)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.AuthResponse{
		AccessToken:  td.AccessToken,
		RefreshToken: td.RefreshToken,
		ExpiresIn:    int64(time.Until(td.AtExpires).Seconds()),
		User: dto.UserResponse{
			ID:           user.ID,
			Email:        user.Email,
			Name:         user.Name,
			WhatsApp:     user.WhatsApp,
			Role:         user.Role,
			Bio:          user.Bio,
			AvatarURL:    user.AvatarURL,
			Address:      user.Address,
			City:         user.City,
			Province:     user.Province,
			Skills:       user.Skills,
			ServiceAreas: user.ServiceAreas,
			CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}

	return response.Success(c, fiber.StatusCreated, resp)
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req dto.LoginRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	identifier := req.Identifier
	if !strings.Contains(identifier, "@") && whatsapp.Validate(identifier) {
		identifier = whatsapp.Normalize(identifier)
	}

	td, user, err := h.authUsecase.Login(c.Context(), identifier, req.Password)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.AuthResponse{
		AccessToken:  td.AccessToken,
		RefreshToken: td.RefreshToken,
		ExpiresIn:    int64(time.Until(td.AtExpires).Seconds()),
		User: dto.UserResponse{
			ID:           user.ID,
			Email:        user.Email,
			Name:         user.Name,
			WhatsApp:     user.WhatsApp,
			Role:         user.Role,
			Bio:          user.Bio,
			AvatarURL:    user.AvatarURL,
			Address:      user.Address,
			City:         user.City,
			Province:     user.Province,
			Skills:       user.Skills,
			ServiceAreas: user.ServiceAreas,
			CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	var req dto.LogoutRequest
	if err := c.Bind().Body(&req); err != nil {
		// ignore body parsing errors — we can still logout from the header token
		_ = err
	}

	accessToken := extractBearerToken(c.Get("Authorization"))

	if err := h.authUsecase.Logout(c.Context(), accessToken, req.RefreshToken); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.SuccessMessage(c, fiber.StatusOK, "logged out successfully")
}

func (h *AuthHandler) Introspect(c fiber.Ctx) error {
	var req dto.IntrospectRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if req.Token == "" {
		resp := dto.IntrospectResponse{Active: false}
		return response.Success(c, fiber.StatusOK, resp)
	}

	user, err := h.authUsecase.Introspect(c.Context(), req.Token)
	if err != nil {
		resp := dto.IntrospectResponse{Active: false}
		return response.Success(c, fiber.StatusOK, resp)
	}

	resp := dto.IntrospectResponse{
		Active: true,
		UserID: user.ID,
		Email:  user.Email,
	}
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *AuthHandler) RefreshToken(c fiber.Ctx) error {
	var req dto.RefreshTokenRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	td, err := h.authUsecase.RefreshToken(c.Context(), req.RefreshToken)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.AuthResponse{
		AccessToken:  td.AccessToken,
		RefreshToken: td.RefreshToken,
		ExpiresIn:    int64(time.Until(td.AtExpires).Seconds()),
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func extractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}
