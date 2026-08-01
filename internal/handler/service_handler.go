package handler

import (
	"context"
	"fmt"
	"math"
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

type ServiceHandler struct {
	serviceUsecase *usecase.ServiceUsecase
	userUsecase    *usecase.ProfileUsecase
	imageResolver  repository.StorageRepository
}

func NewServiceHandler(serviceUsecase *usecase.ServiceUsecase, userUsecase *usecase.ProfileUsecase, imageResolver repository.StorageRepository) *ServiceHandler {
	return &ServiceHandler{
		serviceUsecase: serviceUsecase,
		userUsecase:    userUsecase,
		imageResolver:  imageResolver,
	}
}

func (h *ServiceHandler) Create(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	var req dto.CreateServiceRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	service, err := h.serviceUsecase.CreateService(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toServiceResponse(c.Context(), service, nil, nil)
	return response.Success(c, fiber.StatusCreated, resp)
}

func (h *ServiceHandler) Get(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}

	service, err := h.serviceUsecase.GetService(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	provider, _ := h.userUsecase.GetProfile(c.Context(), service.ProviderID)
	var providerInfo *dto.ProviderInfo
	if provider != nil {
		providerInfo = &dto.ProviderInfo{
			ID:        provider.ID,
			Name:      provider.Name,
			AvatarURL: provider.AvatarURL,
			City:      provider.City,
			Province:  provider.Province,
		}
	}

	stats := h.serviceUsecase.GetReviewStats(c.Context(), service.ID)
	resp := h.toServiceResponse(c.Context(), service, providerInfo, stats)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *ServiceHandler) Update(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}

	var req dto.UpdateServiceRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	service, err := h.serviceUsecase.UpdateService(c.Context(), userID, uint(id), req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := h.toServiceResponse(c.Context(), service, nil, nil)
	return response.Success(c, fiber.StatusOK, resp)
}

func (h *ServiceHandler) List(c fiber.Ctx) error {
	filter, err := parseServiceListFilter(c)
	if err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}

	services, total, err := h.serviceUsecase.ListServices(c.Context(), filter)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	serviceIDs := make([]uint, len(services))
	for i, svc := range services {
		serviceIDs[i] = svc.ID
	}
	statsMap := h.serviceUsecase.GetReviewStatsMap(c.Context(), serviceIDs)

	data := make([]dto.ServiceResponse, len(services))
	for i, svc := range services {
		data[i] = h.toServiceResponse(c.Context(), svc, nil, statsMap[svc.ID])
	}

	totalPages := total / int64(filter.Limit)
	if total%int64(filter.Limit) > 0 {
		totalPages++
	}

	resp := dto.ServiceListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       filter.Page,
			Limit:      filter.Limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}

	return response.Success(c, fiber.StatusOK, resp)
}

func (h *ServiceHandler) GetMyServices(c fiber.Ctx) error {
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

	filter := repository.ServiceFilter{
		Query:    c.Query("q"),
		Category: c.Query("category"),
		Page:     page,
		Limit:    limit,
	}

	services, total, err := h.serviceUsecase.GetMyServices(c.Context(), userID, filter)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	serviceIDs := make([]uint, len(services))
	for i, svc := range services {
		serviceIDs[i] = svc.ID
	}
	statsMap := h.serviceUsecase.GetReviewStatsMap(c.Context(), serviceIDs)

	data := make([]dto.ServiceResponse, len(services))
	for i, svc := range services {
		data[i] = h.toServiceResponse(c.Context(), svc, nil, statsMap[svc.ID])
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	resp := dto.ServiceListResponse{
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

func (h *ServiceHandler) SetStatus(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}

	if err := h.serviceUsecase.SetServiceStatus(c.Context(), userID, uint(id), "inactive"); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.SuccessMessage(c, fiber.StatusOK, "service deactivated successfully")
}

func (h *ServiceHandler) toServiceResponse(_ context.Context, service *entity.Service, provider *dto.ProviderInfo, stats *repository.ServiceReviewStats) dto.ServiceResponse {
	resp := dto.ServiceResponse{
		ID:              service.ID,
		Provider:        provider,
		Title:           service.Title,
		Category:        service.Category,
		Description:     service.Description,
		UnitType:        service.UnitType,
		BasePrice:       service.BasePrice,
		DurationMinutes: service.DurationMinutes,
		ServiceAreas:    service.ServiceAreas,
		ServiceType:     service.ServiceType,
		Status:          service.Status,
		CreatedAt:       service.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       service.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if stats != nil {
		resp.Rating = stats.AverageRating
		resp.ReviewCount = stats.TotalCount
	}

	for _, img := range service.Images {
		resp.Images = append(resp.Images, dto.ServiceImageResponse{
			ID:        img.ID,
			URL:       h.imageResolver.ResolveImageURL(img.URL),
			SortOrder: img.SortOrder,
		})
	}

	for _, addon := range service.AddOns {
		resp.AddOns = append(resp.AddOns, dto.ServiceAddOnResponse{
			ID:    addon.ID,
			Name:  addon.Name,
			Price: addon.Price,
		})
	}

	return resp
}

func parseServiceListFilter(c fiber.Ctx) (repository.ServiceFilter, error) {
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

	filter := repository.ServiceFilter{
		Query:       c.Query("q"),
		Category:    c.Query("category"),
		City:        c.Query("city"),
		Sort:        c.Query("sort", "newest"),
		Page:        page,
		Limit:       limit,
		ServiceType: c.Query("service_type"),
	}

	queryParams := c.Queries()
	minPriceRaw, hasMinPrice := queryParams["min_price"]
	maxPriceRaw, hasMaxPrice := queryParams["max_price"]
	priceRangesRaw, hasPriceRanges := queryParams["price_ranges"]
	if hasPriceRanges && (hasMinPrice || hasMaxPrice) {
		return filter, apierror.NewAPIError(fiber.StatusBadRequest, "PRICE_FILTER_CONFLICT", "price_ranges cannot be combined with min_price or max_price")
	}

	var err error
	if hasMinPrice {
		filter.MinPrice, err = parseNonNegativeInt64(minPriceRaw)
		if err != nil {
			return filter, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_PRICE_FILTER", "min_price must be a non-negative integer")
		}
		filter.MinPriceSet = true
	}
	if hasMaxPrice {
		filter.MaxPrice, err = parseNonNegativeInt64(maxPriceRaw)
		if err != nil {
			return filter, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_PRICE_FILTER", "max_price must be a non-negative integer")
		}
		filter.MaxPriceSet = true
	}
	if hasMinPrice && hasMaxPrice && filter.MinPrice > filter.MaxPrice {
		return filter, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_PRICE_FILTER", "min_price cannot exceed max_price")
	}

	if hasPriceRanges {
		for _, interval := range strings.Split(priceRangesRaw, ",") {
			bounds := strings.Split(interval, "-")
			if len(bounds) != 2 || bounds[0] == "" {
				return filter, invalidPriceRangesError()
			}
			minimum, parseErr := parseNonNegativeInt64(bounds[0])
			if parseErr != nil {
				return filter, invalidPriceRangesError()
			}
			priceRange := repository.PriceRange{Min: minimum}
			if bounds[1] != "" {
				maximum, parseErr := parseNonNegativeInt64(bounds[1])
				if parseErr != nil || minimum > maximum {
					return filter, invalidPriceRangesError()
				}
				priceRange.Max = &maximum
			}
			filter.PriceRanges = append(filter.PriceRanges, priceRange)
		}
	}

	if minRatingRaw, hasMinRating := queryParams["min_rating"]; hasMinRating {
		filter.MinRating, err = strconv.ParseFloat(minRatingRaw, 64)
		if err != nil || math.IsNaN(filter.MinRating) || math.IsInf(filter.MinRating, 0) || filter.MinRating < 1 || filter.MinRating > 5 {
			return filter, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_MIN_RATING", "min_rating must be between 1 and 5")
		}
	}

	return filter, nil
}

func parseNonNegativeInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid non-negative integer")
	}
	return parsed, nil
}

func invalidPriceRangesError() error {
	return apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_PRICE_RANGES", "price_ranges must contain valid inclusive intervals")
}
