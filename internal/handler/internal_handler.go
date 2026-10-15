package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// InternalHandler serves service-to-service reads for sibling backends
// (chat). Guarded by middleware.RequireServiceToken — never by user JWTs.
type InternalHandler struct {
	userRepo repository.UserRepository
}

func NewInternalHandler(userRepo repository.UserRepository) *InternalHandler {
	return &InternalHandler{userRepo: userRepo}
}

// ChatAccessResponse is the per-user chat-access signal consumed by the chat
// backend's live-session enforcement (per-message check in SendMessage plus
// a connect-time check at WS upgrade).
type ChatAccessResponse struct {
	UserID      uint   `json:"user_id"`
	Suspended   bool   `json:"suspended"`
	ChatBlocked bool   `json:"chat_blocked"`
	CheckedAt   string `json:"checked_at"`
}

// ChatAccess returns the block/suspend signal for one user. Unknown users
// are 404; the chat backend treats transport errors as fail-open (logged)
// but explicit suspended/chat_blocked as deny.
func (h *InternalHandler) ChatAccess(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user ID"))
	}
	user, err := h.userRepo.FindByID(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, ChatAccessResponse{
		UserID:      user.ID,
		Suspended:   user.Status == entity.UserStatusSuspended,
		ChatBlocked: user.ChatBlocked,
		CheckedAt:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
}
