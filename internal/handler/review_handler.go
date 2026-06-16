package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

type ReviewHandler struct {
	reviewUsecase *usecase.ReviewUsecase
	userUsecase   *usecase.ProfileUsecase
}

func NewReviewHandler(reviewUsecase *usecase.ReviewUsecase, userUsecase *usecase.ProfileUsecase) *ReviewHandler {
	return &ReviewHandler{
		reviewUsecase: reviewUsecase,
		userUsecase:   userUsecase,
	}
}

func (h *ReviewHandler) Create(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	var req dto.CreateReviewRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	review, err := h.reviewUsecase.CreateReview(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toReviewResponse(c, review)
	return response.Success(c, fiber.StatusCreated, resp)
}

func (h *ReviewHandler) GetServiceReviews(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
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

	reviews, total, stats, err := h.reviewUsecase.GetServiceReviews(c.Context(), uint(id), page, limit)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	data := make([]dto.ReviewResponse, len(reviews))
	for i, r := range reviews {
		data[i] = h.toReviewResponse(c, r)
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	resp := dto.ReviewListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"reviews": resp,
		"stats": dto.ServiceReviewStatsResponse{
			TotalRating:          stats.AverageRating,
			TotalCount:           stats.TotalCount,
			AverageCommunication: stats.AverageCommunication,
			AverageQuality:       stats.AverageQuality,
			AverageValue:         stats.AverageValue,
		},
	})
}

func (h *ReviewHandler) GetProviderReviews(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid provider ID"))
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

	reviews, total, err := h.reviewUsecase.GetProviderReviews(c.Context(), uint(id), page, limit)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	data := make([]dto.ReviewResponse, len(reviews))
	for i, r := range reviews {
		data[i] = h.toReviewResponse(c, r)
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	resp := dto.ReviewListResponse{
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

func (h *ReviewHandler) toReviewResponse(c fiber.Ctx, review *entity.Review) dto.ReviewResponse {
	resp := dto.ReviewResponse{
		ID:                  review.ID,
		Rating:              review.Rating,
		CommunicationRating: review.CommunicationRating,
		QualityRating:       review.QualityRating,
		ValueRating:         review.ValueRating,
		Comment:             review.Comment,
		CreatedAt:           review.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	customer, _ := h.userUsecase.GetProfile(c.Context(), review.CustomerID)
	if customer != nil {
		resp.Customer = dto.ReviewUserInfo{
			ID:        customer.ID,
			Name:      customer.Name,
			AvatarURL: customer.AvatarURL,
		}
	}

	return resp
}
