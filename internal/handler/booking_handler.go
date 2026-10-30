package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

type BookingHandler struct {
	bookingUsecase *usecase.BookingUsecase
	userUsecase    *usecase.ProfileUsecase
	serviceUsecase *usecase.ServiceUsecase
}

func NewBookingHandler(
	bookingUsecase *usecase.BookingUsecase,
	userUsecase *usecase.ProfileUsecase,
	serviceUsecase *usecase.ServiceUsecase,
) *BookingHandler {
	return &BookingHandler{
		bookingUsecase: bookingUsecase,
		userUsecase:    userUsecase,
		serviceUsecase: serviceUsecase,
	}
}

// Create honours the Idempotency-Key header. The contract for clients:
//
//   - A repeat of the same request with the same key returns the booking the
//     first request created: 201 with the same id, and no second booking or
//     checkout.
//   - A repeat can arrive while the first request is still creating its
//     checkout. The booking then comes back with "payment": null. The client
//     must not treat that as an error: it asks POST
//     /api/v1/bookings/:id/payment/retry for the checkout URL. That endpoint
//     reuses the stored key, so the provider answers with the original
//     checkout rather than a second invoice.
//   - A key reused for a different request, or owned by another customer, is
//     refused with 409 IDEMPOTENCY_KEY_REUSED.
func (h *BookingHandler) Create(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	var req dto.CreateBookingRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	booking, err := h.bookingUsecase.CreateBooking(c.Context(), userID, req, idempotencyKey(c))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusCreated, resp)
}

// RetryPayment sends a pending checkout again without creating a booking. It
// is the endpoint a client uses to obtain the checkout URL of a booking whose
// payment has no URL yet, for example after a repeat create returned
// "payment": null. It reuses the stored key, so it never starts a second
// charge.
func (h *BookingHandler) RetryPayment(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	booking, err := h.bookingUsecase.RetryPayment(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

// maxIdempotencyKeyLength bounds the header. A longer key is ignored rather
// than rejected, so the request still goes through without the guarantee.
const maxIdempotencyKeyLength = 64

func idempotencyKey(c fiber.Ctx) string {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if len(key) > maxIdempotencyKeyLength {
		return ""
	}
	return key
}

func (h *BookingHandler) Get(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	booking, err := h.bookingUsecase.GetBooking(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) GetMyBookings(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	role := c.Query("role", "customer")

	filter := repository.BookingFilter{
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	bookings, total, err := h.bookingUsecase.GetMyBookings(c.Context(), userID, role, filter)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	data := make([]dto.BookingResponse, len(bookings))
	for i, b := range bookings {
		data[i] = h.toBookingResponse(c.Context(), b)
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	resp := dto.BookingListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) Accept(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	// Body is optional: direct bookings already carry a schedule, while
	// bid-derived bookings require the provider to supply one here.
	var req dto.AcceptBookingRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&req); err != nil {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
		}
		if validationErrors := validator.Validate(req); validationErrors != nil {
			apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
			apiErr.Details = validationErrors
			return apierror.GlobalErrorHandler(c, apiErr)
		}
	}

	booking, err := h.bookingUsecase.AcceptBooking(c.Context(), userID, uint(id), &req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) Reject(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	booking, err := h.bookingUsecase.RejectBooking(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) Confirm(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	booking, err := h.bookingUsecase.ConfirmBooking(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) Complete(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	booking, err := h.bookingUsecase.CompleteBooking(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) Cancel(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	var req dto.CancelBookingRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "cancel reason is required"))
	}
	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	booking, cancellation, err := h.bookingUsecase.CancelBooking(c.Context(), userID, uint(id), req.Reason, req.Detail)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toBookingResponse(c.Context(), booking)
	if cancellation != nil {
		resp.Cancellation = &dto.CancellationResponse{
			ID:              cancellation.ID,
			BookingID:       cancellation.BookingID,
			CancelledBy:     cancellation.CancelledBy,
			CancelledByRole: cancellation.CancelledByRole,
			Reason:          cancellation.Reason,
			Detail:          cancellation.Detail,
			FeeCharged:      cancellation.FeeCharged,
			PolicyVersion:   cancellation.PolicyVersion,
			CreatedAt:       cancellation.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BookingHandler) GetAvailableSlots(c fiber.Ctx) error {
	providerIDStr := c.Params("id")
	providerID, err := strconv.ParseUint(providerIDStr, 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid provider ID"))
	}

	date := c.Query("date")
	if date == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "MISSING_DATE", "date query parameter is required (YYYY-MM-DD)"))
	}

	slots, err := h.bookingUsecase.GetAvailableSlots(c.Context(), uint(providerID), date)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, map[string]interface{}{
		"date":  date,
		"slots": slots,
	})
}

func (h *BookingHandler) toBookingResponse(ctx context.Context, booking *entity.Booking) dto.BookingResponse {
	resp := dto.BookingResponse{
		ID:             booking.ID,
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
	if booking.ScheduleDate != nil {
		s := booking.ScheduleDate.Format("2006-01-02")
		resp.ScheduleDate = &s
	}
	if booking.CancelledAt != nil {
		s := booking.CancelledAt.Format("2006-01-02T15:04:05Z")
		resp.CancelledAt = &s
	}

	customer, _ := h.userUsecase.GetProfile(ctx, booking.CustomerID)
	if customer != nil {
		resp.Customer = &dto.BookingUserInfo{
			ID:   customer.ID,
			Name: customer.Name,
		}
	}

	service, _ := h.serviceUsecase.GetService(ctx, booking.ServiceID)
	if service != nil {
		resp.Service = &dto.BookingServiceInfo{
			ID:          service.ID,
			Title:       service.Title,
			ServiceType: service.ServiceType,
		}
	}

	provider, _ := h.userUsecase.GetProfile(ctx, booking.ProviderID)
	if provider != nil {
		resp.Provider = &dto.BookingUserInfo{
			ID:   provider.ID,
			Name: provider.Name,
		}
	}

	for _, addon := range booking.BookingAddOns {
		resp.AddOns = append(resp.AddOns, dto.BookingAddOnResponse{
			Name:  addon.Name,
			Price: addon.Price,
		})
	}

	if booking.Payment != nil {
		resp.Payment = toPaymentResponse(booking.Payment)
	}

	return resp
}
