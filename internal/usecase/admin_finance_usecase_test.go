package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ticket 11: finance overview (derived aggregates) + provider-filed
// reimbursements with admin approve/reject as decision-record only.

type financeFakeFinanceRepo struct {
	summary      repository.FinanceSummary
	summaryErr   error
	transactions []repository.FinanceTransaction
	txTotal      int64
	txErr        error
	payouts      []repository.FinancePayout
	payoutsErr   error
	markErr      error
	marked       *entity.PayoutMark
}

func (f *financeFakeFinanceRepo) Summary(ctx context.Context, window repository.FinanceWindow) (repository.FinanceSummary, error) {
	return f.summary, f.summaryErr
}

func (f *financeFakeFinanceRepo) Transactions(ctx context.Context, filter repository.TransactionFilter) ([]repository.FinanceTransaction, int64, error) {
	return f.transactions, f.txTotal, f.txErr
}

func (f *financeFakeFinanceRepo) Payouts(ctx context.Context, period string) ([]repository.FinancePayout, error) {
	return f.payouts, f.payoutsErr
}

func (f *financeFakeFinanceRepo) MarkPayoutPaid(ctx context.Context, mark *entity.PayoutMark) error {
	f.marked = mark
	return f.markErr
}

type financeFakeReimbRepo struct {
	claims  map[uint]*entity.Reimbursement
	nextID  uint
	list    []*entity.Reimbursement
	total   int64
	listErr error
}

func newFinanceFakeReimbRepo() *financeFakeReimbRepo {
	return &financeFakeReimbRepo{claims: map[uint]*entity.Reimbursement{}}
}

func (f *financeFakeReimbRepo) Create(ctx context.Context, r *entity.Reimbursement) error {
	f.nextID++
	r.ID = f.nextID
	r.Status = entity.ReimbursementPending
	r.CreatedAt = time.Now().UTC()
	f.claims[r.ID] = r
	return nil
}

func (f *financeFakeReimbRepo) FindByID(ctx context.Context, id uint) (*entity.Reimbursement, error) {
	if c, ok := f.claims[id]; ok {
		return c, nil
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "REIMBURSEMENT_NOT_FOUND", "reimbursement not found")
}

func (f *financeFakeReimbRepo) List(ctx context.Context, filter repository.ReimbursementFilter) ([]*entity.Reimbursement, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.list, f.total, nil
}

func (f *financeFakeReimbRepo) Decide(ctx context.Context, id uint, decidedBy uint, status string) error {
	c, ok := f.claims[id]
	if !ok || c.Status != entity.ReimbursementPending {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only pending reimbursements can be decided")
	}
	c.Status = status
	c.DecidedBy = &decidedBy
	now := time.Now().UTC()
	c.DecidedAt = &now
	return nil
}

type financeFakeBookingRepo struct {
	bookings map[uint]*entity.Booking
}

func (f *financeFakeBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	if b, ok := f.bookings[id]; ok {
		return b, nil
	}
	return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
}

func newFinanceTestUC() (*AdminFinanceUsecase, *financeFakeFinanceRepo, *financeFakeReimbRepo) {
	finance := &financeFakeFinanceRepo{
		summary: repository.FinanceSummary{
			GMV:             2400000,
			PlatformFees:    160000,
			ByPaymentMethod: map[string]int64{"qris": 2000000, "transfer": 400000},
			CompletedCount:  20,
			CancelledCount:  3,
		},
		payouts: []repository.FinancePayout{
			{ProviderID: 7, Period: "2026-10", Gross: 500000, Fee: 30000, Net: 470000, Status: entity.PayoutPending},
		},
	}
	reimb := newFinanceFakeReimbRepo()
	bookings := &financeFakeBookingRepo{bookings: map[uint]*entity.Booking{
		42: {ID: 42, CustomerID: 10, ProviderID: 7, Status: "completed", Total: 500000, PlatformFee: 30000},
	}}
	return NewAdminFinanceUsecase(finance, reimb, bookings), finance, reimb
}

func TestAdminFinance_GetSummary_ReturnsAggregates(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	summary, err := uc.GetSummary(context.Background(), repository.FinanceWindow{})
	require.NoError(t, err)
	assert.Equal(t, int64(2400000), summary.GMV)
	assert.Equal(t, int64(160000), summary.PlatformFees)
	assert.Equal(t, int64(2000000), summary.ByPaymentMethod["qris"])
	assert.Equal(t, int64(20), summary.CompletedCount)
	assert.Equal(t, int64(3), summary.CancelledCount)
}

func TestAdminFinance_GetSummary_NilMapBecomesEmpty(t *testing.T) {
	finance := &financeFakeFinanceRepo{}
	uc := NewAdminFinanceUsecase(finance, newFinanceFakeReimbRepo(), &financeFakeBookingRepo{})
	summary, err := uc.GetSummary(context.Background(), repository.FinanceWindow{})
	require.NoError(t, err)
	require.NotNil(t, summary.ByPaymentMethod)
}

func TestAdminFinance_GetSummary_PropagatesRepoError(t *testing.T) {
	finance := &financeFakeFinanceRepo{summaryErr: errors.New("db down")}
	uc := NewAdminFinanceUsecase(finance, newFinanceFakeReimbRepo(), &financeFakeBookingRepo{})
	_, err := uc.GetSummary(context.Background(), repository.FinanceWindow{})
	require.Error(t, err)
}

func TestAdminFinance_GetOverview_CombinesAllThree(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	summary, txs, payouts, err := uc.GetOverview(context.Background(), repository.FinanceWindow{}, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2400000), summary.GMV)
	assert.NotNil(t, txs)
	assert.Len(t, payouts, 1)
	assert.Equal(t, int64(470000), payouts[0].Net)
}

func TestAdminFinance_GetOverview_PropagatesPartialFailure(t *testing.T) {
	finance := &financeFakeFinanceRepo{payoutsErr: errors.New("db down")}
	uc := NewAdminFinanceUsecase(finance, newFinanceFakeReimbRepo(), &financeFakeBookingRepo{})
	_, _, _, err := uc.GetOverview(context.Background(), repository.FinanceWindow{}, "2026-10")
	require.Error(t, err)
}

func TestAdminFinance_FileReimbursement_ProviderOwnsBooking(t *testing.T) {
	uc, _, reimb := newFinanceTestUC()
	claim, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)
	assert.Equal(t, entity.ReimbursementPending, claim.Status)
	assert.Equal(t, uint(42), claim.BookingID)
	assert.Equal(t, uint(7), claim.FiledBy)
	stored, _ := reimb.FindByID(context.Background(), claim.ID)
	require.NotNil(t, stored)
}

func TestAdminFinance_FileReimbursement_NonOwnerForbidden(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	_, err := uc.FileReimbursement(context.Background(), 10, 42, 50000, "biaya tol", "")
	require.Error(t, err)
	var derr *apperror.DomainError
	require.ErrorAs(t, err, &derr)
	assert.True(t, errors.Is(derr.Unwrap(), apperror.ErrForbidden))
}

func TestAdminFinance_FileReimbursement_Validation(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	for _, tc := range []struct {
		name      string
		bookingID uint
		amount    int64
		reason    string
	}{
		{"missing booking", 0, 50000, "x"},
		{"zero amount", 42, 0, "x"},
		{"empty reason", 42, 50000, "  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.FileReimbursement(context.Background(), 7, tc.bookingID, tc.amount, tc.reason, "")
			require.Error(t, err)
			var derr *apperror.DomainError
			require.ErrorAs(t, err, &derr)
			assert.Equal(t, "VALIDATION_ERROR", derr.Code)
		})
	}
}

func TestAdminFinance_DecideReimbursement_ApproveTerminal(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	claim, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)
	decided, err := uc.DecideReimbursement(context.Background(), 1, claim.ID, "approved")
	require.NoError(t, err)
	assert.Equal(t, entity.ReimbursementApproved, decided.Status)
	require.NotNil(t, decided.DecidedBy)
	assert.Equal(t, uint(1), *decided.DecidedBy)
}

func TestAdminFinance_DecideReimbursement_DoubleDecide422(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	claim, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)
	_, err = uc.DecideReimbursement(context.Background(), 1, claim.ID, "rejected")
	require.NoError(t, err)
	_, err = uc.DecideReimbursement(context.Background(), 1, claim.ID, "approved")
	require.Error(t, err)
	var derr *apperror.DomainError
	require.ErrorAs(t, err, &derr)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", derr.Code)
	assert.True(t, errors.Is(derr.Unwrap(), apperror.ErrInvalidTransition))
}

func TestAdminFinance_DecideReimbursement_BadDecision400(t *testing.T) {
	uc, _, _ := newFinanceTestUC()
	claim, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)
	_, err = uc.DecideReimbursement(context.Background(), 1, claim.ID, "maybe")
	require.Error(t, err)
	var derr *apperror.DomainError
	require.ErrorAs(t, err, &derr)
	assert.Equal(t, "VALIDATION_ERROR", derr.Code)
}

func TestAdminFinance_MarkPayoutPaid_Validation(t *testing.T) {
	uc, finance, _ := newFinanceTestUC()
	mark, err := uc.MarkPayoutPaid(context.Background(), 1, 7, "2026-10")
	require.NoError(t, err)
	assert.Equal(t, uint(7), mark.ProviderID)
	assert.Equal(t, "2026-10", mark.Period)
	require.NotNil(t, finance.marked)

	_, err = uc.MarkPayoutPaid(context.Background(), 1, 0, "2026-10")
	require.Error(t, err)
	_, err = uc.MarkPayoutPaid(context.Background(), 1, 7, "october")
	require.Error(t, err)
}
