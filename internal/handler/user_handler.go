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
	userUsecase *usecase.UserUsecase
}

func NewUserHandler(userUsecase *usecase.UserUsecase) *UserHandler {
	return &UserHandler{userUsecase: userUsecase}
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
