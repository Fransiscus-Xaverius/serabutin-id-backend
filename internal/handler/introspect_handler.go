package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
)

type IntrospectHandler struct {
	introspectUsecase *usecase.IntrospectUsecase
}

func NewIntrospectHandler(introspectUsecase *usecase.IntrospectUsecase) *IntrospectHandler {
	return &IntrospectHandler{introspectUsecase: introspectUsecase}
}

func (h *IntrospectHandler) Introspect(c fiber.Ctx) error {
	var req dto.IntrospectRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(dto.IntrospectResponse{Active: false})
	}

	result := h.introspectUsecase.Introspect(c.Context(), req.Token)
	return c.JSON(dto.IntrospectResponse{
		Active:       result.Active,
		UserID:       result.UserID,
		Email:        result.Email,
		ChatBlocked:  result.ChatBlocked,
		ChatDisabled: result.ChatDisabled,
	})
}
