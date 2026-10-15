package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

// FinanceRepository serves the read-only admin finance aggregations plus the
// tiny payout-marks table. Money truth is bookings (SubTotal/PlatformFee/
// Total/PaymentMethod); every method issues GROUP BY / SUM / COUNT queries
// only — no row materialization beyond the bounded transaction page.
type FinanceRepository struct {
	db *gorm.DB
}

func NewFinanceRepository(db *gorm.DB) *FinanceRepository {
	return &FinanceRepository{db: db}
}

func applyFinanceWindow(db *gorm.DB, window repository.FinanceWindow) *gorm.DB {
	if !window.From.IsZero() {
		db = db.Where("created_at >= ?", window.From)
	}
	if !window.To.IsZero() {
		db = db.Where("created_at <= ?", window.To)
	}
	return db
}

// Summary aggregates GMV (SUM(total)) and fees (SUM(platform_fee)) over the
// non-cancelled slice of the window, plus a per-payment-method GMV split
// and completed/cancelled counts. Maps are never nil.
func (r *FinanceRepository) Summary(ctx context.Context, window repository.FinanceWindow) (repository.FinanceSummary, error) {
	out := repository.FinanceSummary{ByPaymentMethod: map[string]int64{}}
	type sumRow struct {
		GMV  int64
		Fees int64
	}
	var sums sumRow
	if err := applyFinanceWindow(r.db.WithContext(ctx).Model(&model.BookingModel{}), window).
		Select("COALESCE(SUM(total),0) AS gmv, COALESCE(SUM(platform_fee),0) AS fees").
		Where("status <> ?", "cancelled").
		Scan(&sums).Error; err != nil {
		return out, dbError("sum finance gmv", err)
	}
	out.GMV = sums.GMV
	out.PlatformFees = sums.Fees

	type methodRow struct {
		PaymentMethod string
		Total         int64
	}
	var methods []methodRow
	if err := applyFinanceWindow(r.db.WithContext(ctx).Model(&model.BookingModel{}), window).
		Select("payment_method, COALESCE(SUM(total),0) AS total").
		Where("status <> ?", "cancelled").
		Group("payment_method").Scan(&methods).Error; err != nil {
		return out, dbError("sum finance by payment method", err)
	}
	for _, row := range methods {
		out.ByPaymentMethod[row.PaymentMethod] = row.Total
	}

	if err := applyFinanceWindow(r.db.WithContext(ctx).Model(&model.BookingModel{}), window).
		Where("status = ?", "completed").Count(&out.CompletedCount).Error; err != nil {
		return out, dbError("count completed bookings", err)
	}
	if err := applyFinanceWindow(r.db.WithContext(ctx).Model(&model.BookingModel{}), window).
		Where("status = ?", "cancelled").Count(&out.CancelledCount).Error; err != nil {
		return out, dbError("count cancelled bookings", err)
	}
	return out, nil
}

// Transactions lists bookings newest-first as money rows with optional
// status / payment-method filters inside the window.
func (r *FinanceRepository) Transactions(ctx context.Context, filter repository.TransactionFilter) ([]repository.FinanceTransaction, int64, error) {
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	scope := applyFinanceWindow(r.db.WithContext(ctx).Model(&model.BookingModel{}), filter.Window)
	if strings.TrimSpace(filter.Status) != "" {
		scope = scope.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.PaymentMethod) != "" {
		scope = scope.Where("payment_method = ?", strings.TrimSpace(filter.PaymentMethod))
	}
	var total int64
	if err := scope.Count(&total).Error; err != nil {
		return nil, 0, dbError("count transactions", err)
	}
	var rows []model.BookingModel
	if err := scope.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, dbError("list transactions", err)
	}
	out := make([]repository.FinanceTransaction, len(rows))
	for i, b := range rows {
		out[i] = repository.FinanceTransaction{
			ID:            b.ID,
			CustomerID:    b.CustomerID,
			ProviderID:    b.ProviderID,
			ServiceID:     b.ServiceID,
			SubTotal:      b.SubTotal,
			PlatformFee:   b.PlatformFee,
			Total:         b.Total,
			PaymentMethod: b.PaymentMethod,
			Status:        b.Status,
			CreatedAt:     b.CreatedAt,
		}
	}
	return out, total, nil
}

// Payouts sums per-provider gross (SUM(total)) and fee
// (SUM(platform_fee)) over completed bookings created in the YYYY-MM
// period. A manual payout mark flips that provider's row to paid.
func (r *FinanceRepository) Payouts(ctx context.Context, period string) ([]repository.FinancePayout, error) {
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "period must be YYYY-MM")
	}
	end := start.AddDate(0, 1, 0)
	type payoutRow struct {
		ProviderID uint
		Gross      int64
		Fee        int64
	}
	var rows []payoutRow
	if err := r.db.WithContext(ctx).Model(&model.BookingModel{}).
		Select("provider_id, COALESCE(SUM(total),0) AS gross, COALESCE(SUM(platform_fee),0) AS fee").
		Where("status = ? AND created_at >= ? AND created_at < ?", "completed", start, end).
		Group("provider_id").Order("provider_id ASC").Scan(&rows).Error; err != nil {
		return nil, dbError("sum payouts", err)
	}
	var marks []model.PayoutMarkModel
	if err := r.db.WithContext(ctx).Where("period = ?", period).Find(&marks).Error; err != nil {
		return nil, dbError("list payout marks", err)
	}
	paid := map[uint]bool{}
	for _, m := range marks {
		paid[m.ProviderID] = true
	}
	out := make([]repository.FinancePayout, len(rows))
	for i, row := range rows {
		status := entity.PayoutPending
		if paid[row.ProviderID] {
			status = entity.PayoutPaid
		}
		out[i] = repository.FinancePayout{
			ProviderID: row.ProviderID,
			Period:     period,
			Gross:      row.Gross,
			Fee:        row.Fee,
			Net:        row.Gross - row.Fee,
			Status:     status,
		}
	}
	return out, nil
}

// MarkPayoutPaid records the manual paid mark (idempotent on repeat marks).
// It never moves money — the decision record is the whole effect.
func (r *FinanceRepository) MarkPayoutPaid(ctx context.Context, mark *entity.PayoutMark) error {
	m := &model.PayoutMarkModel{
		ProviderID: mark.ProviderID,
		Period:     mark.Period,
		MarkedBy:   mark.MarkedBy,
		MarkedAt:   mark.MarkedAt,
	}
	if err := r.db.WithContext(ctx).
		Where("provider_id = ? AND period = ?", mark.ProviderID, mark.Period).
		Attrs(*m).FirstOrCreate(m).Error; err != nil {
		return dbError("mark payout paid", err)
	}
	mark.ID = m.ID
	mark.CreatedAt = m.CreatedAt
	return nil
}

// ReimbursementRepository persists provider-filed reimbursement claims.
type ReimbursementRepository struct {
	db *gorm.DB
}

func NewReimbursementRepository(db *gorm.DB) *ReimbursementRepository {
	return &ReimbursementRepository{db: db}
}

func toReimbursementEntity(m *model.ReimbursementModel) *entity.Reimbursement {
	return &entity.Reimbursement{
		ID:          m.ID,
		BookingID:   m.BookingID,
		FiledBy:     m.FiledBy,
		Amount:      m.Amount,
		Reason:      m.Reason,
		EvidenceURL: m.EvidenceURL,
		Status:      m.Status,
		DecidedBy:   m.DecidedBy,
		DecidedAt:   m.DecidedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func (r *ReimbursementRepository) Create(ctx context.Context, re *entity.Reimbursement) error {
	m := &model.ReimbursementModel{
		BookingID:   re.BookingID,
		FiledBy:     re.FiledBy,
		Amount:      re.Amount,
		Reason:      re.Reason,
		EvidenceURL: re.EvidenceURL,
		Status:      entity.ReimbursementPending,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return dbError("create reimbursement", err)
	}
	re.ID = m.ID
	re.Status = m.Status
	re.CreatedAt = m.CreatedAt
	re.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ReimbursementRepository) FindByID(ctx context.Context, id uint) (*entity.Reimbursement, error) {
	var m model.ReimbursementModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "REIMBURSEMENT_NOT_FOUND", "reimbursement not found")
		}
		return nil, dbError("find reimbursement", err)
	}
	return toReimbursementEntity(&m), nil
}

// List returns the queue newest-first with optional status filter.
func (r *ReimbursementRepository) List(ctx context.Context, filter repository.ReimbursementFilter) ([]*entity.Reimbursement, int64, error) {
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	scope := r.db.WithContext(ctx).Model(&model.ReimbursementModel{})
	if strings.TrimSpace(filter.Status) != "" {
		scope = scope.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	var total int64
	if err := scope.Count(&total).Error; err != nil {
		return nil, 0, dbError("count reimbursements", err)
	}
	var rows []model.ReimbursementModel
	if err := scope.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, dbError("list reimbursements", err)
	}
	out := make([]*entity.Reimbursement, len(rows))
	for i := range rows {
		out[i] = toReimbursementEntity(&rows[i])
	}
	return out, total, nil
}

// Decide flips a pending row to its terminal status with the decider
// attached. The update is conditional on status=pending so a concurrent
// second decision affects zero rows instead of overwriting.
func (r *ReimbursementRepository) Decide(ctx context.Context, id uint, decidedBy uint, status string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&model.ReimbursementModel{}).
		Where("id = ? AND status = ?", id, entity.ReimbursementPending).
		Updates(map[string]interface{}{
			"status":     status,
			"decided_by": decidedBy,
			"decided_at": now,
		})
	if res.Error != nil {
		return dbError("decide reimbursement", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only pending reimbursements can be decided")
	}
	return nil
}
