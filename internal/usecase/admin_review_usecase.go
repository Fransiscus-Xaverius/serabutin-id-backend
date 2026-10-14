package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// AdminReviewUsecase owns admin review-moderation reads plus the
// review-visibility and report-resolution mutations. Single-row writes go
// through column-scoped repository updates (no multi-repo transaction
// needed); every mutation is audit-logged by
// middleware.AdminAuditLogger, so the handler enforces a mandatory `reason`
// and this layer never writes audit rows directly.
type AdminReviewUsecase struct {
	reviewRepo repository.ReviewRepository
	reportRepo repository.ReportRepository
}

func NewAdminReviewUsecase(reviewRepo repository.ReviewRepository, reportRepo repository.ReportRepository) *AdminReviewUsecase {
	return &AdminReviewUsecase{reviewRepo: reviewRepo, reportRepo: reportRepo}
}

// ListReviews serves the admin reviews queue (newest first, hidden rows
// included). Read-only.
func (uc *AdminReviewUsecase) ListReviews(ctx context.Context, filter repository.AdminReviewFilter) ([]*entity.Review, int64, error) {
	reviews, total, err := uc.reviewRepo.ListForAdmin(ctx, filter)
	if err != nil {
		log.Printf("[AdminReviewUsecase.ListReviews] list reviews error: %v", err)
		return nil, 0, err
	}
	return reviews, total, nil
}

// SetVisibility hides (or unhides) a review. Hiding never deletes the row;
// public reads and aggregates exclude hidden rows. Setting the current flag
// is an idempotent success without a write.
func (uc *AdminReviewUsecase) SetVisibility(ctx context.Context, id uint, hidden bool) (*entity.Review, error) {
	review, err := uc.reviewRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminReviewUsecase.SetVisibility] find review error: %v", err)
		return nil, err
	}
	if review.Hidden == hidden {
		return review, nil
	}
	if err := uc.reviewRepo.SetHidden(ctx, id, hidden); err != nil {
		log.Printf("[AdminReviewUsecase.SetVisibility] set hidden error: %v", err)
		return nil, err
	}
	review.Hidden = hidden
	return review, nil
}

// ListReports serves the admin report queue (newest first). Read-only.
func (uc *AdminReviewUsecase) ListReports(ctx context.Context, filter repository.ReportFilter) ([]*entity.Report, int64, error) {
	reports, total, err := uc.reportRepo.ListForAdmin(ctx, filter)
	if err != nil {
		log.Printf("[AdminReviewUsecase.ListReports] list reports error: %v", err)
		return nil, 0, err
	}
	return reports, total, nil
}

// ResolveReport moves an open report to resolved|dismissed (terminal).
// Unknown targets are rejected with VALIDATION_ERROR (400); resolving a
// non-open report returns INVALID_STATUS_TRANSITION (422).
func (uc *AdminReviewUsecase) ResolveReport(ctx context.Context, adminID uint, id uint, status string) (*entity.Report, error) {
	switch status {
	case entity.ReportStatusResolved, entity.ReportStatusDismissed:
	default:
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "status must be resolved or dismissed")
	}
	report, err := uc.reportRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminReviewUsecase.ResolveReport] find report error: %v", err)
		return nil, err
	}
	if report.Status != entity.ReportStatusOpen {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only open reports can be resolved")
	}
	if err := uc.reportRepo.Resolve(ctx, id, adminID, status); err != nil {
		log.Printf("[AdminReviewUsecase.ResolveReport] resolve error: %v", err)
		return nil, err
	}
	report.Status = status
	report.ResolvedBy = &adminID
	return report, nil
}
