package handler

import (
	"context"
	"strconv"
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

// SetAdminBroadcastUC attaches the broadcast usecase. Kept as a setter
// (mirroring the review/stats/chat pattern) so the established constructor
// shape — relied on by existing handler tests — stays stable.
func (h *AdminHandler) SetAdminBroadcastUC(uc *usecase.AdminBroadcastUsecase) {
	h.adminBroadcastUC = uc
}

// CreateBroadcast composes an instant or scheduled blast. Mandatory
// title/body/audience (400); a past scheduled_at is 422. Instant blasts
// send synchronously in-request. The admin audit middleware persists the
// action automatically. Admin-only.
func (h *AdminHandler) CreateBroadcast(c fiber.Ctx) error {
	var payload dto.AdminBroadcastCreateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "title, body, and audience are required"))
	}
	var scheduledAt *time.Time
	if s := strings.TrimSpace(ptrOrEmpty(payload.ScheduledAt)); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "scheduled_at must be RFC 3339"))
		}
		scheduledAt = &parsed
	}
	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()
	b, err := h.adminBroadcastUC.CreateBroadcast(ctx, requestctx.GetUserID(c), payload.Title, payload.Body, payload.Audience, scheduledAt)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusCreated, toAdminBroadcastResponse(b))
}

// ListBroadcasts returns the paginated broadcast history, newest first.
// Read-only.
func (h *AdminHandler) ListBroadcasts(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, total, err := h.adminBroadcastUC.ListBroadcasts(ctx, page, limit)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.AdminBroadcastResponse, len(rows))
	for i, b := range rows {
		data[i] = toAdminBroadcastResponse(b)
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}
	return response.Success(c, fiber.StatusOK, dto.AdminBroadcastListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// EstimateBroadcastRecipients counts active recipients for an audience so
// the compose confirm modal can show the blast size. Read-only.
func (h *AdminHandler) EstimateBroadcastRecipients(c fiber.Ctx) error {
	audience := strings.TrimSpace(c.Query("audience"))
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	total, err := h.adminBroadcastUC.EstimateRecipients(ctx, audience)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.AdminBroadcastEstimateResponse{
		Audience:       audience,
		RecipientCount: total,
	})
}

// CancelBroadcast moves a scheduled broadcast to cancelled (422 otherwise).
// The audit middleware persists the action automatically. Admin-only.
func (h *AdminHandler) CancelBroadcast(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid broadcast ID"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	b, err := h.adminBroadcastUC.CancelBroadcast(ctx, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toAdminBroadcastResponse(b))
}

func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02T15:04:05Z")
	return &s
}

func toAdminBroadcastResponse(b *entity.Broadcast) dto.AdminBroadcastResponse {
	return dto.AdminBroadcastResponse{
		ID:             b.ID,
		Title:          b.Title,
		Body:           b.Body,
		Audience:       b.Audience,
		Status:         b.Status,
		ScheduledAt:    fmtTimePtr(b.ScheduledAt),
		RecipientCount: b.RecipientCount,
		SentAt:         fmtTimePtr(b.SentAt),
		CreatedBy:      b.CreatedBy,
		CreatedAt:      b.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
