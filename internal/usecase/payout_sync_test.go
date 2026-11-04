package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

// syncFixture releases booking 42 so that its payout is submitted under PO-42.
func syncFixture(t *testing.T) (*releaseFixture, *entity.Payout) {
	t.Helper()
	f := newReleaseFixture(t)
	payout, err := f.uc.Release(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	return f, payout
}

func TestPayoutUsecase_SyncTransferStatus_TakesTheOutcomeFromDOKUFailed(t *testing.T) {
	f, payout := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed, LatestStatusDesc: "rejected by bank"}

	got, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusFailed, got.Status)
	stored := f.payouts.rows[payout.ID]
	assert.Equal(t, entity.PayoutStatusFailed, stored.Status)
	assert.Nil(t, stored.PaidAt)
	assert.Contains(t, stored.LastError, "status 06")
	assert.Equal(t, []string{"PO-42"}, f.gateway.statuses)
}

func TestPayoutUsecase_SyncTransferStatus_TakesTheOutcomeFromDOKUSuccess(t *testing.T) {
	f, payout := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}

	got, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, got.Status)
	stored := f.payouts.rows[payout.ID]
	assert.Equal(t, entity.PayoutStatusPaid, stored.Status)
	assert.NotNil(t, stored.PaidAt)
}

func TestPayoutUsecase_SyncTransferStatus_PendingKeepsItSubmitted(t *testing.T) {
	f, payout := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusPending}

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusSubmitted, f.payouts.rows[payout.ID].Status)
}

func TestPayoutUsecase_SyncTransferStatus_PaidPayoutIsUnchangedAndDOKUIsNotAsked(t *testing.T) {
	f, payout := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)
	before := *f.payouts.rows[payout.ID]
	queriesBefore := len(f.gateway.statuses)

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed}
	got, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, got.Status)
	assert.Equal(t, before, *f.payouts.rows[payout.ID])
	assert.Len(t, f.gateway.statuses, queriesBefore, "a paid payout must not be re-checked against DOKU")
}

func TestPayoutUsecase_SyncTransferStatus_FailedThenPaidWhenDOKUSaysSo(t *testing.T) {
	f, payout := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed}
	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err = f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, f.payouts.rows[payout.ID].Status)
}

func TestPayoutUsecase_SyncTransferStatus_StatusErrorChangesNothing(t *testing.T) {
	f, payout := syncFixture(t)
	before := *f.payouts.rows[payout.ID]
	f.gateway.statusErr = errors.New("doku unavailable")

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.Error(t, err)
	assert.Equal(t, before, *f.payouts.rows[payout.ID])
}

func TestPayoutUsecase_SyncTransferStatus_UnknownDOKUCodeChangesNothing(t *testing.T) {
	f, payout := syncFixture(t)
	before := *f.payouts.rows[payout.ID]
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: "99"}

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.Error(t, err)
	assert.Equal(t, before, *f.payouts.rows[payout.ID])
}

func TestPayoutUsecase_SyncTransferStatus_UnknownReferenceIsNotFoundAndAsksNothing(t *testing.T) {
	f, _ := syncFixture(t)

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-UNKNOWN")

	assert.True(t, errors.Is(err, apperror.ErrNotFound))
	assert.Empty(t, f.gateway.statuses)
}

func TestPayoutUsecase_SyncTransferStatus_RefusesWithoutReleaseDependencies(t *testing.T) {
	uc := NewPayoutUsecase(nil, nil, nil, "")

	_, err := uc.SyncTransferStatus(context.Background(), "PO-42")

	require.Error(t, err)
}
