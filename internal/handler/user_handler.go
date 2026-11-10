package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

type UserHandler struct {
	userUsecase   *usecase.UserUsecase
	statsUsecase  *usecase.StatsUsecase
	reviewUsecase *usecase.ReviewUsecase
}

func NewUserHandler(userUsecase *usecase.UserUsecase, statsUsecase *usecase.StatsUsecase, reviewUsecase *usecase.ReviewUsecase) *UserHandler {
	return &UserHandler{
		userUsecase:   userUsecase,
		statsUsecase:  statsUsecase,
		reviewUsecase: reviewUsecase,
	}
}

func (h *UserHandler) GetPublicProfile(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user id")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	user, err := h.userUsecase.GetPublicProfile(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.PublicUserResponse{
		ID:        user.ID,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *UserHandler) GetStats(c fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user id")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	stats, err := h.statsUsecase.GetUserStats(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, stats)
}
