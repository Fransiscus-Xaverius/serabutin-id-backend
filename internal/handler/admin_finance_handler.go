package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// SetAdminFinanceUC attaches the finance usecase. Kept as a setter
// (mirroring the review/stats/chat pattern) so the established constructor
// shape — relied on by existing handler tests — stays stable.
func (h *AdminHandler) SetAdminFinanceUC(uc *usecase.AdminFinanceUsecase) {
	h.adminFinanceUC = uc
}

// parseFinanceWindow parses optional ?from=&to= (YYYY-MM-DD, inclusive)
// bounds against bookings.created_at. Bad values are VALIDATION_ERROR
// (400).
func parseFinanceWindow(c fiber.Ctx) (repository.FinanceWindow, error) {
	var window repository.FinanceWindow
	if from := strings.TrimSpace(c.Query("from")); from != "" {
		t, err := time.Parse("2006-01-02", from)
		if err != nil {
			return window, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "from must be YYYY-MM-DD")
		}
		window.From = t.UTC()
	}
	if to := strings.TrimSpace(c.Query("to")); to != "" {
		t, err := time.Parse("2006-01-02", to)
		if err != nil {
			return window, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "to must be YYYY-MM-DD")
		}
		window.To = t.UTC().Add(24*time.Hour - time.Second)
	}
	return window, nil
}

func toFinanceSummaryDTO(s repository.FinanceSummary) dto.FinanceSummaryResponse {
	if s.ByPaymentMethod == nil {
		s.ByPaymentMethod = map[string]int64{}
	}
	return dto.FinanceSummaryResponse{
		GMV:             s.GMV,
		PlatformFees:    s.PlatformFees,
		ByPaymentMethod: s.ByPaymentMethod,
		CompletedCount:  s.CompletedCount,
		CancelledCount:  s.CancelledCount,
	}
}

func toFinanceTransactionRow(t repository.FinanceTransaction) dto.FinanceTransactionRow {
	return dto.FinanceTransactionRow{
		ID:            t.ID,
		CustomerID:    t.CustomerID,
		ProviderID:    t.ProviderID,
		ServiceID:     t.ServiceID,
		SubTotal:      t.SubTotal,
		PlatformFee:   t.PlatformFee,
		Total:         t.Total,
		PaymentMethod: t.PaymentMethod,
		Status:        t.Status,
		CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toFinancePayoutRow(p repository.FinancePayout) dto.FinancePayoutRow {
	return dto.FinancePayoutRow{
		ProviderID: p.ProviderID,
		Period:     p.Period,
		Gross:      p.Gross,
		Fee:        p.Fee,
		Net:        p.Net,
		Status:     p.Status,
	}
}

func toReimbursementRow(r *entity.Reimbursement) dto.ReimbursementRow {
	row := dto.ReimbursementRow{
		ID:          r.ID,
		BookingID:   r.BookingID,
		FiledBy:     r.FiledBy,
		Amount:      r.Amount,
		Reason:      r.Reason,
		EvidenceURL: r.EvidenceURL,
		Status:      r.Status,
		DecidedBy:   r.DecidedBy,
		CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.DecidedAt != nil {
		s := r.DecidedAt.Format("2006-01-02T15:04:05Z")
		row.DecidedAt = &s
	}
	return row
}

// GetFinanceOverview returns the combined money view for the Keuangan tab:
// summary aggregates plus the latest 20 transactions and the current-period
// payouts. Read-only, never audit-logged.
func (h *AdminHandler) GetFinanceOverview(c fiber.Ctx) error {
	window, err := parseFinanceWindow(c)
	if err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	summary, transactions, payouts, err := h.adminFinanceUC.GetOverview(ctx, window, strings.TrimSpace(c.Query("period")))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	txRows := make([]dto.FinanceTransactionRow, len(transactions))
	for i, t := range transactions {
		txRows[i] = toFinanceTransactionRow(t)
	}
	payoutRows := make([]dto.FinancePayoutRow, len(payouts))
	for i, p := range payouts {
		payoutRows[i] = toFinancePayoutRow(p)
	}
	return response.Success(c, fiber.StatusOK, dto.FinanceOverviewResponse{
		Summary:      toFinanceSummaryDTO(summary),
		Transactions: txRows,
		Payouts:      payoutRows,
	})
}

// GetFinanceSummary returns the finance-shaped aggregates for an optional
// ?from=&to= window. Read-only.
func (h *AdminHandler) GetFinanceSummary(c fiber.Ctx) error {
	window, err := parseFinanceWindow(c)
	if err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	summary, err := h.adminFinanceUC.GetSummary(ctx, window)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toFinanceSummaryDTO(summary))
}

// ListTransactions returns the paginated booking-derived money rows. Query:
// status, payment_method, from/to (YYYY-MM-DD), page, limit (clamped to 50,
// following the existing admin list pattern). Read-only.
func (h *AdminHandler) ListTransactions(c fiber.Ctx) error {
	window, err := parseFinanceWindow(c)
	if err != nil {
		return apierror.GlobalErrorHandler(c, err)
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	rows, total, err := h.adminFinanceUC.ListTransactions(c.Context(), repository.TransactionFilter{
		Status:        strings.TrimSpace(c.Query("status")),
		PaymentMethod: strings.TrimSpace(c.Query("payment_method")),
		Window:        window,
		Page:          page,
		Limit:         limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.FinanceTransactionRow, len(rows))
	for i, t := range rows {
		data[i] = toFinanceTransactionRow(t)
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
	return response.Success(c, fiber.StatusOK, dto.FinanceTransactionListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ListPayouts returns the per-provider computed earnings for ?period=
// (YYYY-MM, defaults to the current UTC month). Read-only.
func (h *AdminHandler) ListPayouts(c fiber.Ctx) error {
	rows, err := h.adminFinanceUC.ListPayouts(c.Context(), strings.TrimSpace(c.Query("period")))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.FinancePayoutRow, len(rows))
	for i, p := range rows {
		data[i] = toFinancePayoutRow(p)
	}
	return response.Success(c, fiber.StatusOK, data)
}

// MarkPayoutPaid manually marks one provider period as paid.
// `provider_id`, `period`, and `reason` are all mandatory; the audit
// middleware persists the reason automatically. No money moves — the mark
// is the whole effect.
func (h *AdminHandler) MarkPayoutPaid(c fiber.Ctx) error {
	var payload dto.FinancePayoutMarkRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "provider_id, period and reason are required"))
	}
	if payload.ProviderID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "provider_id is required"))
	}
	if strings.TrimSpace(payload.Period) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "period is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	mark, err := h.adminFinanceUC.MarkPayoutPaid(c.Context(), requestctx.GetUserID(c), payload.ProviderID, strings.TrimSpace(payload.Period))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, map[string]interface{}{
		"provider_id": mark.ProviderID,
		"period":      mark.Period,
		"status":      entity.PayoutPaid,
	})
}

// ListReimbursements returns the paginated claim queue, newest first.
// Query: status (pending|approved|rejected), page, limit (clamped to 50,
// following the existing admin list pattern). Read-only.
func (h *AdminHandler) ListReimbursements(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	rows, total, err := h.adminFinanceUC.ListReimbursements(c.Context(), repository.ReimbursementFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.ReimbursementRow, len(rows))
	for i, r := range rows {
		data[i] = toReimbursementRow(r)
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
	return response.Success(c, fiber.StatusOK, dto.ReimbursementListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// DecideReimbursement approves/rejects a pending claim (terminal).
// `decision` and `reason` are both mandatory; the audit middleware persists
// the reason automatically. Re-deciding returns 422. Approval records the
// decision only — no money movement in v1.
func (h *AdminHandler) DecideReimbursement(c fiber.Ctx) error {
	var payload dto.ReimbursementDecideRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "decision and reason are required"))
	}
	if strings.TrimSpace(payload.Decision) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "decision is required"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid reimbursement ID"))
	}
	claim, err := h.adminFinanceUC.DecideReimbursement(c.Context(), requestctx.GetUserID(c), uint(id), strings.TrimSpace(payload.Decision))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toReimbursementRow(claim))
}

// ReimbursementHandler exposes the user-facing filing endpoint. Kept
// separate from AdminHandler because it lives outside the /admin group
// (authenticated provider, not admin).
type ReimbursementHandler struct {
	financeUC *usecase.AdminFinanceUsecase
}

func NewReimbursementHandler(financeUC *usecase.AdminFinanceUsecase) *ReimbursementHandler {
	return &ReimbursementHandler{financeUC: financeUC}
}

// File lets the booking's provider file a reimbursement claim linked to
// that booking. `booking_id`, `amount`, and `reason` are mandatory;
// filing against someone else's booking returns 403.
func (h *ReimbursementHandler) File(c fiber.Ctx) error {
	var payload dto.ReimbursementFileRequest
	if err := c.Bind().JSON(&payload); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "booking_id, amount and reason are required"))
	}
	if payload.BookingID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "booking_id is required"))
	}
	if payload.Amount <= 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "amount must be positive"))
	}
	if strings.TrimSpace(payload.Reason) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "reason is required"))
	}
	claim, err := h.financeUC.FileReimbursement(c.Context(), requestctx.GetUserID(c), payload.BookingID, payload.Amount, strings.TrimSpace(payload.Reason), strings.TrimSpace(payload.EvidenceURL))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusCreated, toReimbursementRow(claim))
}
