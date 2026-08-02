package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

type ProfileHandler struct {
	profileUsecase *usecase.ProfileUsecase
}

func NewProfileHandler(profileUsecase *usecase.ProfileUsecase) *ProfileHandler {
	return &ProfileHandler{profileUsecase: profileUsecase}
}

func (h *ProfileHandler) GetProfile(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	user, err := h.profileUsecase.GetProfile(c.Context(), userID)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.UserResponse{
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
		BudgetMin:    user.BudgetMin,
		BudgetMax:    user.BudgetMax,
		CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *ProfileHandler) UpdateProfile(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		apiErr := apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	var req dto.UpdateProfileRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if err := req.Validate(); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_BUDGET_RANGE", err.Error())
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	user, err := h.profileUsecase.UpdateProfile(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.UserResponse{
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
		BudgetMin:    user.BudgetMin,
		BudgetMax:    user.BudgetMax,
		CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return response.Success(c, fiber.StatusOK, resp)
}
