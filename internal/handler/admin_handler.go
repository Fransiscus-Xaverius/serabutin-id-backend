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
)

// AdminHandler exposes read-only JSON for the admin UI.
type AdminHandler struct {
	cancelRepo     repository.CancellationRepository
	notificationUC *usecase.NotificationUsecase
	bookingRepo    repository.BookingRepository
	userUsecase    *usecase.ProfileUsecase
	serviceUsecase *usecase.ServiceUsecase
	auditRepo      repository.AuditLogRepository
	adminUserUC    *usecase.AdminUserUsecase
	adminServiceUC *usecase.AdminServiceUsecase
	imageResolver  repository.StorageRepository
}

func NewAdminHandler(
	cancelRepo repository.CancellationRepository,
	notificationUC *usecase.NotificationUsecase,
	bookingRepo repository.BookingRepository,
	userUsecase *usecase.ProfileUsecase,
	serviceUsecase *usecase.ServiceUsecase,
	auditRepo repository.AuditLogRepository,
	adminUserUC *usecase.AdminUserUsecase,
	adminServiceUC *usecase.AdminServiceUsecase,
	imageResolver repository.StorageRepository,
) *AdminHandler {
	return &AdminHandler{
		cancelRepo:     cancelRepo,
		notificationUC: notificationUC,
		bookingRepo:    bookingRepo,
		userUsecase:    userUsecase,
		serviceUsecase: serviceUsecase,
		auditRepo:      auditRepo,
		adminUserUC:    adminUserUC,
		adminServiceUC: adminServiceUC,
		imageResolver:  imageResolver,
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

// ListAuditLogs returns paginated audit rows, newest first, optionally
// filtered by actor (?actor_id=) and action (?action=). Admin-only.
func (h *AdminHandler) ListAuditLogs(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	actorID, _ := strconv.ParseUint(c.Query("actor_id", "0"), 10, 64)
	items, total, err := h.auditRepo.List(c.Context(), repository.AuditLogFilter{
		ActorUserID: uint(actorID),
		Action:      c.Query("action"),
		Page:        page,
		Limit:       limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.AuditLogResponse, len(items))
	for i, x := range items {
		data[i] = dto.AuditLogResponse{
			ID:          x.ID,
			ActorUserID: x.ActorUserID,
			Action:      x.Action,
			TargetType:  x.TargetType,
			TargetID:    x.TargetID,
			Reason:      x.Reason,
			CreatedAt:   x.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
	return response.Success(c, fiber.StatusOK, dto.AuditLogListResponse{
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
	var scheduleDate *string
	if booking.ScheduleDate != nil {
		s := booking.ScheduleDate.Format("2006-01-02")
		scheduleDate = &s
	}
	resp := dto.BookingResponse{
		ID:             booking.ID,
		ScheduleDate:   scheduleDate,
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

// ListUsers returns the paginated admin user directory. Query: q (partial
// match over email/name/whatsapp), email, whatsapp, role, status, page,
// limit (clamped to 50, following the existing admin list pattern).
func (h *AdminHandler) ListUsers(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	users, total, err := h.adminUserUC.ListUsers(c.Context(), repository.UserFilter{
		Q:        strings.TrimSpace(c.Query("q")),
		Email:    strings.TrimSpace(c.Query("email")),
		WhatsApp: strings.TrimSpace(c.Query("whatsapp")),
		Role:     strings.TrimSpace(c.Query("role")),
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		Limit:    limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.AdminUserResponse, len(users))
	for i, u := range users {
		data[i] = dto.AdminUserResponse{
			ID:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			WhatsApp:  u.WhatsApp,
			Role:      u.Role,
			Status:    u.Status,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
	return response.Success(c, fiber.StatusOK, dto.AdminUserListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetUser returns a single admin-directory row.
func (h *AdminHandler) GetUser(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user ID"))
	}
	user, err := h.adminUserUC.GetUser(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.AdminUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		WhatsApp:  user.WhatsApp,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// adminMutationReason extracts the mandatory `reason` from the request body.
// Every admin user mutation is audit-logged by middleware.AdminAuditLogger,
// which persists this same field — the handler only enforces presence.
func adminMutationReason(c fiber.Ctx) string {
	var payload dto.AdminReasonRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Reason)
}

func requireMutationReason(c fiber.Ctx) (string, error) {
	reason := adminMutationReason(c)
	if reason == "" {
		return "", apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required")
	}
	return reason, nil
}

// SuspendUser sets users.status=suspended (idempotent 200 on no-op).
func (h *AdminHandler) SuspendUser(c fiber.Ctx) error {
	if _, err := requireMutationReason(c); err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user ID"))
	}
	user, err := h.adminUserUC.Suspend(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.AdminUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		WhatsApp:  user.WhatsApp,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// RestoreUser sets users.status=active (idempotent 200 on no-op).
func (h *AdminHandler) RestoreUser(c fiber.Ctx) error {
	if _, err := requireMutationReason(c); err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user ID"))
	}
	user, err := h.adminUserUC.Restore(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.AdminUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		WhatsApp:  user.WhatsApp,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// SetUserRole changes users.role. The route-level RequireAdmin guarantees the
// actor is an admin; the usecase rejects self-demotion.
func (h *AdminHandler) SetUserRole(c fiber.Ctx) error {
	var payload dto.AdminRoleRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "role and reason are required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	if strings.TrimSpace(payload.Role) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "role is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid user ID"))
	}
	user, err := h.adminUserUC.SetRole(c.Context(), requestctx.GetUserID(c), uint(id), strings.TrimSpace(payload.Role))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, dto.AdminUserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		WhatsApp:  user.WhatsApp,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// ListServices returns the paginated moderation queue. Query: q (partial
// match over title/description/category), category, status (comma-separated
// allowlist, e.g. `draft` or `draft,pending`; empty means all), page, limit
// (clamped to 50, following the existing admin list pattern). Bypasses the
// public active-only gate via the admin usecase.
func (h *AdminHandler) ListServices(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	services, total, err := h.adminServiceUC.ListServices(c.Context(), repository.ServiceFilter{
		Query:    strings.TrimSpace(c.Query("q")),
		Category: strings.TrimSpace(c.Query("category")),
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		Limit:    limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.AdminServiceRow, len(services))
	for i, s := range services {
		data[i] = dto.AdminServiceRow{
			ID:         s.ID,
			ProviderID: s.ProviderID,
			Title:      s.Title,
			Category:   s.Category,
			BasePrice:  s.BasePrice,
			Status:     s.Status,
			CreatedAt:  s.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
	return response.Success(c, fiber.StatusOK, dto.AdminServiceListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetService returns one listing in any status with provider context
// (bypasses the public active-only gate). Read-only.
func (h *AdminHandler) GetService(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}
	service, err := h.adminServiceUC.GetService(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, h.toAdminServiceDetail(c.Context(), service))
}

// SetServiceStatus applies one moderation transition (approve / reject /
// takedown / restore). `status` and `reason` are both mandatory; the audit
// middleware persists the reason automatically.
func (h *AdminHandler) SetServiceStatus(c fiber.Ctx) error {
	var payload dto.AdminServiceStatusRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "status and reason are required"))
	}
	if strings.TrimSpace(payload.Status) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "status is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid service ID"))
	}
	service, err := h.adminServiceUC.SetServiceStatus(c.Context(), uint(id), strings.TrimSpace(payload.Status))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, h.toAdminServiceDetail(c.Context(), service))
}

func (h *AdminHandler) toAdminServiceDetail(ctx context.Context, service *entity.Service) dto.AdminServiceDetailResponse {
	resp := dto.AdminServiceDetailResponse{
		ID:              service.ID,
		ProviderID:      service.ProviderID,
		Title:           service.Title,
		Category:        service.Category,
		Description:     service.Description,
		UnitType:        service.UnitType,
		BasePrice:       service.BasePrice,
		BudgetMin:       service.BudgetMin,
		BudgetMax:       service.BudgetMax,
		AutoAcceptPrice: service.AutoAcceptPrice,
		DurationMinutes: service.DurationMinutes,
		ServiceAreas:    service.ServiceAreas,
		ServiceType:     service.ServiceType,
		Status:          service.Status,
		CreatedAt:       service.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if h.userUsecase != nil {
		if provider, _ := h.userUsecase.GetProfile(ctx, service.ProviderID); provider != nil {
			resp.Provider = &dto.AdminServiceProvider{ID: provider.ID, Name: provider.Name}
		}
	}
	for _, img := range service.Images {
		url := img.URL
		if h.imageResolver != nil {
			url = h.imageResolver.ResolveImageURL(img.URL)
		}
		resp.Images = append(resp.Images, dto.ServiceImageResponse{
			ID:        img.ID,
			URL:       url,
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
