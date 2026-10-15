package usecase

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"golang.org/x/sync/errgroup"
)

// AdminFinanceUsecase owns the read-only money views (derived from bookings
// — no ledger exists in v1) plus the reimbursement lifecycle. Approval is a
// decision record only: it never moves money. Every admin mutation is
// audit-logged by middleware.AdminAuditLogger, so the handler enforces the
// mandatory `reason` and this layer never writes audit rows directly.
type AdminFinanceUsecase struct {
	financeRepo repository.FinanceRepository
	reimbRepo   repository.ReimbursementRepository
	bookingRepo FinanceBookingReader
}

// FinanceBookingReader is the narrow booking surface the reimbursement
// filing needs (provider-owns-booking check). The full
// repository.BookingRepository satisfies it.
type FinanceBookingReader interface {
	FindByID(ctx context.Context, id uint) (*entity.Booking, error)
}

func NewAdminFinanceUsecase(
	financeRepo repository.FinanceRepository,
	reimbRepo repository.ReimbursementRepository,
	bookingRepo FinanceBookingReader,
) *AdminFinanceUsecase {
	return &AdminFinanceUsecase{
		financeRepo: financeRepo,
		reimbRepo:   reimbRepo,
		bookingRepo: bookingRepo,
	}
}

// GetSummary returns the finance-shaped aggregates for the window.
func (uc *AdminFinanceUsecase) GetSummary(ctx context.Context, window repository.FinanceWindow) (repository.FinanceSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	summary, err := uc.financeRepo.Summary(ctx, window)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.GetSummary] summary error: %v", err)
		return summary, err
	}
	if summary.ByPaymentMethod == nil {
		summary.ByPaymentMethod = map[string]int64{}
	}
	return summary, nil
}

// ListTransactions serves the booking-derived money rows.
func (uc *AdminFinanceUsecase) ListTransactions(ctx context.Context, filter repository.TransactionFilter) ([]repository.FinanceTransaction, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, total, err := uc.financeRepo.Transactions(ctx, filter)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.ListTransactions] list error: %v", err)
		return nil, 0, err
	}
	return rows, total, nil
}

// ListPayouts serves the per-provider computed earnings for the YYYY-MM
// period. Empty period defaults to the current UTC month.
func (uc *AdminFinanceUsecase) ListPayouts(ctx context.Context, period string) ([]repository.FinancePayout, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if strings.TrimSpace(period) == "" {
		period = time.Now().UTC().Format("2006-01")
	}
	rows, err := uc.financeRepo.Payouts(ctx, strings.TrimSpace(period))
	if err != nil {
		log.Printf("[AdminFinanceUsecase.ListPayouts] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

// GetOverview fans out summary + latest transactions + current-period
// payouts with errgroup over a 10s timeout. The lists are bounded (latest
// 20 transactions, current-month payouts) so the tab renders in one round
// trip; full pagination lives on the granular endpoints.
func (uc *AdminFinanceUsecase) GetOverview(ctx context.Context, window repository.FinanceWindow, period string) (repository.FinanceSummary, []repository.FinanceTransaction, []repository.FinancePayout, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if strings.TrimSpace(period) == "" {
		period = time.Now().UTC().Format("2006-01")
	} else {
		period = strings.TrimSpace(period)
	}

	var summary repository.FinanceSummary
	var transactions []repository.FinanceTransaction
	var payouts []repository.FinancePayout

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		summary, err = uc.financeRepo.Summary(gctx, window)
		if err != nil {
			log.Printf("[AdminFinanceUsecase.GetOverview] summary error: %v", err)
		}
		return err
	})
	g.Go(func() error {
		var err error
		var total int64
		transactions, total, err = uc.financeRepo.Transactions(gctx, repository.TransactionFilter{Window: window, Page: 1, Limit: 20})
		_ = total
		if err != nil {
			log.Printf("[AdminFinanceUsecase.GetOverview] transactions error: %v", err)
		}
		return err
	})
	g.Go(func() error {
		var err error
		payouts, err = uc.financeRepo.Payouts(gctx, period)
		if err != nil {
			log.Printf("[AdminFinanceUsecase.GetOverview] payouts error: %v", err)
		}
		return err
	})
	if err := g.Wait(); err != nil {
		return summary, nil, nil, err
	}
	if summary.ByPaymentMethod == nil {
		summary.ByPaymentMethod = map[string]int64{}
	}
	if transactions == nil {
		transactions = []repository.FinanceTransaction{}
	}
	if payouts == nil {
		payouts = []repository.FinancePayout{}
	}
	return summary, transactions, payouts, nil
}

// MarkPayoutPaid records the manual paid mark for one provider period.
// Unknown/empty periods are rejected with VALIDATION_ERROR (400). F12: a
// provider with no computed earnings in the period is rejected with
// VALIDATION_ERROR (400) instead of recording a phantom mark against an
// empty period. No money moves — the mark is the whole effect, and the
// response echoes the recorded mark visibly (provider_id/period/status).
func (uc *AdminFinanceUsecase) MarkPayoutPaid(ctx context.Context, adminID uint, providerID uint, period string) (*entity.PayoutMark, error) {
	if providerID == 0 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "provider_id is required")
	}
	period = strings.TrimSpace(period)
	if _, err := time.Parse("2006-01", period); err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "period must be YYYY-MM")
	}
	rows, err := uc.financeRepo.Payouts(ctx, period)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.MarkPayoutPaid] payouts lookup error: %v", err)
		return nil, err
	}
	found := false
	for _, row := range rows {
		if row.ProviderID == providerID && row.Gross > 0 {
			found = true
			break
		}
	}
	if !found {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "provider has no earnings in period")
	}
	now := time.Now().UTC()
	mark := &entity.PayoutMark{ProviderID: providerID, Period: period, MarkedBy: adminID, MarkedAt: now}
	if err := uc.financeRepo.MarkPayoutPaid(ctx, mark); err != nil {
		log.Printf("[AdminFinanceUsecase.MarkPayoutPaid] mark error: %v", err)
		return nil, err
	}
	return mark, nil
}

// FileReimbursement files a provider claim linked to their booking. Only
// the booking's provider may file (anyone else gets FORBIDDEN → 403).
// Amount must be positive and reason non-empty (VALIDATION_ERROR → 400).
// F11: a second filing is rejected (DUPLICATE_CLAIM → 409) while a pending
// claim exists for the same booking — the provider must wait for the
// approve/reject decision first.
func (uc *AdminFinanceUsecase) FileReimbursement(ctx context.Context, filerID uint, bookingID uint, amount int64, reason string, evidenceURL string) (*entity.Reimbursement, error) {
	if bookingID == 0 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "booking_id is required")
	}
	if amount <= 0 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "amount must be positive")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "reason is required")
	}
	booking, err := uc.bookingRepo.FindByID(ctx, bookingID)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.FileReimbursement] find booking error: %v", err)
		return nil, err
	}
	if booking.ProviderID != filerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the booking provider can file a reimbursement")
	}
	if pending, err := uc.reimbRepo.FindPendingByBooking(ctx, bookingID); err != nil {
		var domainErr *apperror.DomainError
		if !(errors.As(err, &domainErr) && domainErr.Code == "REIMBURSEMENT_NOT_FOUND") {
			log.Printf("[AdminFinanceUsecase.FileReimbursement] pending lookup error: %v", err)
			return nil, err
		}
	} else if pending != nil {
		return nil, apperror.NewDomainError(apperror.ErrAlreadyExists, "DUPLICATE_CLAIM", "a pending reimbursement already exists for this booking")
	}
	claim := &entity.Reimbursement{
		BookingID:   bookingID,
		FiledBy:     filerID,
		Amount:      amount,
		Reason:      strings.TrimSpace(reason),
		EvidenceURL: strings.TrimSpace(evidenceURL),
	}
	if err := uc.reimbRepo.Create(ctx, claim); err != nil {
		log.Printf("[AdminFinanceUsecase.FileReimbursement] create error: %v", err)
		return nil, err
	}
	return claim, nil
}

// ListReimbursements serves the admin claim queue (newest first).
func (uc *AdminFinanceUsecase) ListReimbursements(ctx context.Context, filter repository.ReimbursementFilter) ([]*entity.Reimbursement, int64, error) {
	rows, total, err := uc.reimbRepo.List(ctx, filter)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.ListReimbursements] list error: %v", err)
		return nil, 0, err
	}
	return rows, total, nil
}

// DecideReimbursement moves a pending claim to approved|rejected (terminal).
// Unknown decisions are rejected with VALIDATION_ERROR (400); deciding a
// non-pending claim returns INVALID_STATUS_TRANSITION (422). Approval
// records the decision only — no money movement in v1.
func (uc *AdminFinanceUsecase) DecideReimbursement(ctx context.Context, adminID uint, id uint, decision string) (*entity.Reimbursement, error) {
	switch strings.TrimSpace(decision) {
	case entity.ReimbursementDecisionApproved, entity.ReimbursementDecisionRejected:
	default:
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "decision must be approved or rejected")
	}
	claim, err := uc.reimbRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminFinanceUsecase.DecideReimbursement] find error: %v", err)
		return nil, err
	}
	if claim.Status != entity.ReimbursementPending {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only pending reimbursements can be decided")
	}
	status := strings.TrimSpace(decision)
	if err := uc.reimbRepo.Decide(ctx, id, adminID, status); err != nil {
		log.Printf("[AdminFinanceUsecase.DecideReimbursement] decide error: %v", err)
		return nil, err
	}
	now := time.Now().UTC()
	claim.Status = status
	claim.DecidedBy = &adminID
	claim.DecidedAt = &now
	return claim, nil
}
