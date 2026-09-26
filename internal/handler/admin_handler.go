package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// AdminHandler exposes read-only JSON for the future admin UI (ticket 07).
// No pages in this phase; role guard is applied at the route group level.
type AdminHandler struct {
	cancelRepo     repository.CancellationRepository
	notificationUC *usecase.NotificationUsecase
}

func NewAdminHandler(cancelRepo repository.CancellationRepository, notificationUC *usecase.NotificationUsecase) *AdminHandler {
	return &AdminHandler{cancelRepo: cancelRepo, notificationUC: notificationUC}
}

func (h *AdminHandler) ListCancellations(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	bookingID, _ := strconv.ParseUint(c.Query("booking_id", "0"), 10, 64)
	items, total, err := h.cancelRepo.List(c.Context(), repository.CancellationFilter{
		BookingID: uint(bookingID),
		Role:      c.Query("role"),
		Page:      page,
		Limit:     limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.CancellationResponse, len(items))
	for i, x := range items {
		data[i] = dto.CancellationResponse{
			ID:              x.ID,
			CancelledBy:     x.CancelledBy,
			CancelledByRole: x.CancelledByRole,
			Reason:          x.Reason,
			Detail:          x.Detail,
			FeeCharged:      x.FeeCharged,
			PolicyVersion:   x.PolicyVersion,
			CreatedAt:       x.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
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
	return response.Success(c, fiber.StatusOK, map[string]interface{}{
		"data": data,
		"pagination": dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *AdminHandler) ListNotifications(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	items, total, err := h.notificationUC.ListAll(c.Context(), page, limit, c.Query("type"))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.NotificationResponse, len(items))
	for i, n := range items {
		data[i] = toNotificationResponse(n)
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
	return response.Success(c, fiber.StatusOK, dto.NotificationListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
