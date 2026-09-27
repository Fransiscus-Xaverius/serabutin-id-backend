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

// AdminHandler exposes read-only JSON for the admin UI.
// Role guard is applied at the route group level.
type AdminHandler struct {
	cancelRepo     repository.CancellationRepository
	notificationUC *usecase.NotificationUsecase
	bookingRepo    repository.BookingRepository
	userUsecase    *usecase.ProfileUsecase
	serviceUsecase *usecase.ServiceUsecase
}

func NewAdminHandler(
	cancelRepo repository.CancellationRepository,
	notificationUC *usecase.NotificationUsecase,
	bookingRepo repository.BookingRepository,
	userUsecase *usecase.ProfileUsecase,
	serviceUsecase *usecase.ServiceUsecase,
) *AdminHandler {
	return &AdminHandler{
		cancelRepo:     cancelRepo,
		notificationUC: notificationUC,
		bookingRepo:    bookingRepo,
		userUsecase:    userUsecase,
		serviceUsecase: serviceUsecase,
	}
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
			BookingID:       x.BookingID,
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

// GetBooking returns full booking context for admins (bypasses the
// participant check of the user-facing endpoint). Read-only.
func (h *AdminHandler) GetBooking(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}
	booking, err := h.bookingRepo.FindByID(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	resp := dto.BookingResponse{
		ID:             booking.ID,
		ScheduleDate:   booking.ScheduleDate.Format("2006-01-02"),
		ScheduleTime:   booking.ScheduleTime,
		Address:        booking.Address,
		Notes:          booking.Notes,
		SubTotal:       booking.SubTotal,
		PlatformFee:    booking.PlatformFee,
		Total:          booking.Total,
		PaymentMethod:  booking.PaymentMethod,
		Status:         booking.Status,
		CancellationID: booking.CancellationID,
		CancelledBy:    booking.CancelledBy,
		CreatedAt:      booking.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if booking.CancelledAt != nil {
		s := booking.CancelledAt.Format("2006-01-02T15:04:05Z")
		resp.CancelledAt = &s
	}
	if customer, _ := h.userUsecase.GetProfile(c.Context(), booking.CustomerID); customer != nil {
		resp.Customer = &dto.BookingUserInfo{ID: customer.ID, Name: customer.Name}
	}
	if provider, _ := h.userUsecase.GetProfile(c.Context(), booking.ProviderID); provider != nil {
		resp.Provider = &dto.BookingUserInfo{ID: provider.ID, Name: provider.Name}
	}
	if service, _ := h.serviceUsecase.GetService(c.Context(), booking.ServiceID); service != nil {
		resp.Service = &dto.BookingServiceInfo{ID: service.ID, Title: service.Title}
	}
	for _, addon := range booking.BookingAddOns {
		resp.AddOns = append(resp.AddOns, dto.BookingAddOnResponse{Name: addon.Name, Price: addon.Price})
	}
	return response.Success(c, fiber.StatusOK, resp)
}
