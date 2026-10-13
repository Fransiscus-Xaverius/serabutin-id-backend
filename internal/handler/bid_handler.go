package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

type BidHandler struct {
	bidUsecase    *usecase.BidUsecase
	userUsecase   *usecase.ProfileUsecase
	serviceUsecase *usecase.ServiceUsecase
}

func NewBidHandler(bidUsecase *usecase.BidUsecase, userUsecase *usecase.ProfileUsecase, serviceUsecase *usecase.ServiceUsecase) *BidHandler {
	return &BidHandler{
		bidUsecase:    bidUsecase,
		userUsecase:   userUsecase,
		serviceUsecase: serviceUsecase,
	}
}

func (h *BidHandler) Create(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	var req dto.CreateBidRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	bid, err := h.bidUsecase.CreateBid(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.BidResponse{
		ID:        bid.ID,
		ServiceID: bid.ServiceID,
		Price:     bid.Price,
		Notes:     bid.Notes,
		Status:    bid.Status,
		CreatedAt: bid.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: bid.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return response.Success(c, fiber.StatusCreated, resp)
}

func (h *BidHandler) GetByService(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	serviceID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}

	bids, err := h.bidUsecase.GetBidsForService(c.Context(), userID, uint(serviceID))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	data := make([]dto.BidResponse, len(bids))
	for i, bid := range bids {
		provider, _ := h.userUsecase.GetProfile(c.Context(), bid.ProviderID)
		resp := dto.BidResponse{
			ID:        bid.ID,
			ServiceID: bid.ServiceID,
			Price:     bid.Price,
			Notes:     bid.Notes,
			Status:    bid.Status,
			CreatedAt: bid.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: bid.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if provider != nil {
			resp.Provider = &dto.BookingUserInfo{ID: provider.ID, Name: provider.Name}
		}
		data[i] = resp
	}

	return response.Success(c, fiber.StatusOK, data)
}

func (h *BidHandler) GetMyBids(c fiber.Ctx) error {
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

	filter := repository.BidFilter{
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	bids, total, counts, err := h.bidUsecase.GetMyBids(c.Context(), userID, filter)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	data := make([]dto.BidResponse, len(bids))
	for i, bid := range bids {
		service, _ := h.serviceUsecase.GetService(c.Context(), bid.ServiceID)
		resp := dto.BidResponse{
			ID:        bid.ID,
			ServiceID: bid.ServiceID,
			Price:     bid.Price,
			Notes:     bid.Notes,
			Status:    bid.Status,
			CreatedAt: bid.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: bid.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if service != nil {
			resp.ServiceTitle = service.Title
			resp.ServiceCategory = service.Category
			resp.ServiceDescription = service.Description
			resp.ServiceLocation = service.ServiceAreas
			resp.ServiceType = service.ServiceType
			if len(service.Images) > 0 {
				resp.ServiceImage = service.Images[0].URL
			}
			if customer, err := h.userUsecase.GetProfile(c.Context(), service.ProviderID); err == nil {
				resp.Customer = &dto.BookingUserInfo{
					ID:   customer.ID,
					Name: customer.Name,
				}
			}
		}
		data[i] = resp
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	return response.Success(c, fiber.StatusOK, dto.BidListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
		Counts: dto.BidStatusCounts{
			Pending:  counts["pending"],
			Accepted: counts["accepted"],
			Rejected: counts["rejected"],
		},
	})
}

func (h *BidHandler) Accept(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	bidID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid bid ID"))
	}

	booking, err := h.bidUsecase.AcceptBid(c.Context(), userID, uint(bidID))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.BookingResponse{
		ID:            booking.ID,
		ScheduleTime:  booking.ScheduleTime,
		SubTotal:      booking.SubTotal,
		PlatformFee:   booking.PlatformFee,
		Total:         booking.Total,
		PaymentMethod: booking.PaymentMethod,
		Status:        booking.Status,
		CreatedAt:     booking.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if booking.ScheduleDate != nil {
		s := booking.ScheduleDate.Format("2006-01-02")
		resp.ScheduleDate = &s
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *BidHandler) Reject(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	bidID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid bid ID"))
	}

	if err := h.bidUsecase.RejectBid(c.Context(), userID, uint(bidID)); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.SuccessMessage(c, fiber.StatusOK, "bid rejected")
}
