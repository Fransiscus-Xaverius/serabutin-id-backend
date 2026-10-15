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
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// SetAdminCMSUC attaches the CMS usecase. Kept as a setter (mirroring the
// broadcast/review/stats/chat pattern) so the established constructor shape
// — relied on by existing handler tests — stays stable.
func (h *AdminHandler) SetAdminCMSUC(uc *usecase.AdminCMSUsecase) {
	h.adminCMSUC = uc
}

func toBannerResponse(b *entity.Banner) dto.BannerResponse {
	return dto.BannerResponse{
		ID:        b.ID,
		Title:     b.Title,
		ImageURL:  b.ImageURL,
		LinkURL:   b.LinkURL,
		Placement: b.Placement,
		SortOrder: b.SortOrder,
		Active:    b.Active,
		StartsAt:  fmtTimePtr(b.StartsAt),
		EndsAt:    fmtTimePtr(b.EndsAt),
		CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toCategoryResponse(c *entity.Category) dto.CategoryResponse {
	return dto.CategoryResponse{
		ID:        c.ID,
		Slug:      c.Slug,
		Name:      c.Name,
		Icon:      c.Icon,
		SortOrder: c.SortOrder,
		Active:    c.Active,
		CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ListBanners returns every banner, newest first (drafts and expired rows
// included). Read-only.
func (h *AdminHandler) ListBanners(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, err := h.adminCMSUC.ListBannersForAdmin(ctx)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.BannerResponse, len(rows))
	for i, b := range rows {
		data[i] = toBannerResponse(b)
	}
	return response.Success(c, fiber.StatusOK, dto.BannerListResponse{Data: data})
}

// CreateBanner creates a banner. `title`, `image_url`, and `reason` are
// mandatory; `placement` defaults to `home`, `active` defaults to true. The
// audit middleware persists the reason automatically. Admin-only.
func (h *AdminHandler) CreateBanner(c fiber.Ctx) error {
	var payload dto.AdminBannerCreateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "title, image_url, and reason are required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	startsAt, err := parseOptionalTime(payload.StartsAt)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "starts_at must be RFC 3339"))
	}
	endsAt, err := parseOptionalTime(payload.EndsAt)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "ends_at must be RFC 3339"))
	}
	active := true
	if payload.Active != nil {
		active = *payload.Active
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	b, err := h.adminCMSUC.CreateBanner(ctx, payload.Title, payload.ImageURL, payload.LinkURL, payload.Placement, payload.SortOrder, active, startsAt, endsAt)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusCreated, toBannerResponse(b))
}

// UpdateBanner applies a partial banner update (edit, toggle active,
// publish, reschedule). `reason` is mandatory; the audit middleware persists
// it automatically. Admin-only.
func (h *AdminHandler) UpdateBanner(c fiber.Ctx) error {
	var payload dto.AdminBannerUpdateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid banner ID"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	b, err := h.adminCMSUC.UpdateBanner(ctx, uint(id), usecase.BannerPatch{
		Title:     payload.Title,
		ImageURL:  payload.ImageURL,
		LinkURL:   payload.LinkURL,
		Placement: payload.Placement,
		SortOrder: payload.SortOrder,
		Active:    payload.Active,
		StartsAt:  payload.StartsAt,
		EndsAt:    payload.EndsAt,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toBannerResponse(b))
}

// DeleteBanner hard-deletes a banner. `reason` is mandatory — accepted via
// JSON body or `?reason=` query (DELETEs often carry no body). The audit
// middleware persists it automatically. Admin-only.
func (h *AdminHandler) DeleteBanner(c fiber.Ctx) error {
	reason := deleteMutationReason(c)
	if reason == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid banner ID"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	if err := h.adminCMSUC.DeleteBanner(ctx, uint(id)); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, map[string]interface{}{"id": uint(id)})
}

// ListCategories returns the full taxonomy ordered by sort_order, id
// (inactive rows included). Read-only.
func (h *AdminHandler) ListCategories(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	rows, err := h.adminCMSUC.ListCategoriesForAdmin(ctx)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.CategoryResponse, len(rows))
	for i, cat := range rows {
		data[i] = toCategoryResponse(cat)
	}
	return response.Success(c, fiber.StatusOK, dto.CategoryListResponse{Data: data})
}

// CreateCategory creates a taxonomy row. `name` and `reason` are mandatory;
// an empty slug is auto-derived from the name. Duplicate slugs are 409
// CATEGORY_EXISTS. Admin-only.
func (h *AdminHandler) CreateCategory(c fiber.Ctx) error {
	var payload dto.AdminCategoryCreateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "name and reason are required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	active := true
	if payload.Active != nil {
		active = *payload.Active
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	cat, err := h.adminCMSUC.CreateCategory(ctx, payload.Slug, payload.Name, payload.Icon, payload.SortOrder, active)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusCreated, toCategoryResponse(cat))
}

// UpdateCategory applies a partial category update (rename, reorder,
// deactivate). `reason` is mandatory. Deactivating a row with live services
// is allowed — services keep their old value. Admin-only.
func (h *AdminHandler) UpdateCategory(c fiber.Ctx) error {
	var payload dto.AdminCategoryUpdateRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid category ID"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	cat, err := h.adminCMSUC.UpdateCategory(ctx, uint(id), usecase.CategoryPatch{
		Slug:      payload.Slug,
		Name:      payload.Name,
		Icon:      payload.Icon,
		SortOrder: payload.SortOrder,
		Active:    payload.Active,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toCategoryResponse(cat))
}

// DeleteCategory hard-deletes a taxonomy row. `reason` is mandatory —
// accepted via JSON body or `?reason=` query. Existing services keep their
// old value; providers can no longer pick the slug (422 UNKNOWN_CATEGORY).
// Admin-only.
func (h *AdminHandler) DeleteCategory(c fiber.Ctx) error {
	reason := deleteMutationReason(c)
	if reason == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid category ID"))
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	if err := h.adminCMSUC.DeleteCategory(ctx, uint(id)); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, map[string]interface{}{"id": uint(id)})
}

// deleteMutationReason reads the mandatory `reason` for bodyless-friendly
// DELETEs: JSON body first, then the `?reason=` query param.
func deleteMutationReason(c fiber.Ctx) string {
	var payload dto.AdminReasonRequest
	if err := c.Bind().JSON(&payload); err == nil && strings.TrimSpace(payload.Reason) != "" {
		return strings.TrimSpace(payload.Reason)
	}
	return strings.TrimSpace(c.Query("reason"))
}

func parseOptionalTime(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
