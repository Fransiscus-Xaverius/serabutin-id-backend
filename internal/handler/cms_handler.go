package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// CmsHandler serves the PUBLIC storefront reads for ticket 10: visible
// banners and the active taxonomy. No auth; never audit-logged.
type CmsHandler struct {
	cmsUC *usecase.AdminCMSUsecase
}

func NewCmsHandler(cmsUC *usecase.AdminCMSUsecase) *CmsHandler {
	return &CmsHandler{cmsUC: cmsUC}
}

// ListBanners returns active banners in their schedule window for a
// placement (default `home`), ordered by sort_order, id.
func (h *CmsHandler) ListBanners(c fiber.Ctx) error {
	placement := strings.TrimSpace(c.Query("placement", "home"))
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, err := h.cmsUC.ListPublicBanners(ctx, placement)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.BannerResponse, len(rows))
	for i, b := range rows {
		data[i] = toBannerResponse(b)
	}
	return response.Success(c, fiber.StatusOK, dto.BannerListResponse{Data: data})
}

// ListCategories returns active categories ordered by sort_order, id.
func (h *CmsHandler) ListCategories(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, err := h.cmsUC.ListPublicCategories(ctx)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.CategoryResponse, len(rows))
	for i, cat := range rows {
		data[i] = toCategoryResponse(cat)
	}
	return response.Success(c, fiber.StatusOK, dto.CategoryListResponse{Data: data})
}
