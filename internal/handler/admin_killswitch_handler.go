package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// SetKillSwitchUC attaches the kill-switch usecase. Kept as a setter
// (mirroring the review/stats/chat pattern) so the established constructor
// shape — relied on by existing handler tests — stays stable.
func (h *AdminHandler) SetKillSwitchUC(uc *usecase.KillSwitchUsecase) {
	h.killSwitchUC = uc
}

// ListKillSwitches returns the durable toggle states for all modules,
// master first. Read-only (never audit-logged). Admin-only.
func (h *AdminHandler) ListKillSwitches(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, err := h.killSwitchUC.List(ctx)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.KillSwitchListResponse{Data: toKillSwitchRows(rows)})
}

// SetKillSwitch flips one module. `enabled` and `reason` are both
// mandatory; toggling `master` additionally requires the fat-finger guard
// `confirm` ('FREEZE' to disable, 'UNFREEZE' to enable). The audit
// middleware persists the reason automatically. Admin-only.
func (h *AdminHandler) SetKillSwitch(c fiber.Ctx) error {
	var payload dto.KillSwitchUpdateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "enabled and reason are required"))
	}
	if payload.Enabled == nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "enabled is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	module := strings.ToLower(strings.TrimSpace(c.Params("module")))
	if !entity.IsValidKillSwitchModule(module) {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusNotFound, "KILL_SWITCH_NOT_FOUND", "unknown module"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	sw, err := h.killSwitchUC.SetEnabled(ctx, requestctx.GetUserID(c), module, *payload.Enabled, payload.Reason, payload.Confirm)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toKillSwitchRow(sw))
}

func toKillSwitchRow(sw *entity.KillSwitch) dto.KillSwitchResponse {
	return dto.KillSwitchResponse{
		Module:    sw.Module,
		Enabled:   sw.Enabled,
		UpdatedBy: sw.UpdatedBy,
		UpdatedAt: sw.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Reason:    sw.Reason,
	}
}

func toKillSwitchRows(rows []*entity.KillSwitch) []dto.KillSwitchResponse {
	data := make([]dto.KillSwitchResponse, len(rows))
	for i, sw := range rows {
		data[i] = toKillSwitchRow(sw)
	}
	return data
}
