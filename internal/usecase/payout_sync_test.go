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
	requireStateUnchangedButAttempted(t, before, *f.payouts.rows[payout.ID])
}

func TestPayoutUsecase_SyncTransferStatus_UnknownDOKUCodeChangesNothing(t *testing.T) {
	f, payout := syncFixture(t)
	before := *f.payouts.rows[payout.ID]
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: "99"}

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.Error(t, err)
	requireStateUnchangedButAttempted(t, before, *f.payouts.rows[payout.ID])
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

// A refund notification names DOKU's transactionId, not the partner
// reference. A refunded answer reopens a paid payout so the money is not
// reported as delivered after it came back.
func TestPayoutUsecase_SyncRefundStatus_ReopensAPaidPayout(t *testing.T) {
	f, payout := syncFixture(t)
	ctx := context.Background()
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err := f.uc.SyncTransferStatus(ctx, "PO-42")
	require.NoError(t, err)
	require.Equal(t, entity.PayoutStatusPaid, f.payouts.rows[payout.ID].Status)
	paidAt := f.payouts.rows[payout.ID].PaidAt

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusRefunded}
	got, err := f.uc.SyncRefundStatus(ctx, "PAY-1")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusFailed, got.Status)
	assert.Equal(t, paidAt, got.PaidAt)
	assert.Contains(t, got.LastError, "status 04")
}

// A refund notification can arrive before DOKU's check-status reflects it.
// The paid state and its original paid_at stay unchanged.
func TestPayoutUsecase_SyncRefundStatus_KeepsPaidWhenDOKUStillReportsSuccess(t *testing.T) {
	f, payout := syncFixture(t)
	ctx := context.Background()
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	_, err := f.uc.SyncTransferStatus(ctx, "PO-42")
	require.NoError(t, err)
	paidAt := f.payouts.rows[payout.ID].PaidAt

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusSuccess}
	got, err := f.uc.SyncRefundStatus(ctx, "PAY-1")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusPaid, got.Status)
	assert.Equal(t, paidAt, got.PaidAt)
}

func TestPayoutUsecase_SyncRefundStatus_UnknownTransactionIsNotFound(t *testing.T) {
	f, _ := syncFixture(t)

	_, err := f.uc.SyncRefundStatus(context.Background(), "DK-UNKNOWN")

	requireDomainCode(t, err, "PAYOUT_NOT_FOUND")
}

// A transfer response can be lost after DOKU accepted the payment. Check-status
// carries DOKU's reference, so a later refund notification can still resolve
// the payout instead of being acknowledged as unknown.
func TestPayoutUsecase_SyncRefundStatus_ResolvesAReferenceLearnedFromCheckStatus(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	require.NoError(t, f.payouts.Create(ctx, &entity.Payout{
		BookingID: 99, ProviderID: 7, Amount: 150000, Currency: payoutCurrency, Channel: payoutChannel,
		Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-99",
		BeneficiaryBankCode: "014", BeneficiaryAccountNumberEncrypted: "enc:1234567890",
		BeneficiaryAccountHolderNameEncrypted: "enc:Siti Rahayu", BeneficiaryLast4: "7890",
		BeneficiaryFingerprint: "keyed-fp",
	}))
	f.gateway.statusResult = &doku.TransactionStatusResult{
		PartnerReferenceNo: "PO-99", OriginalReferenceNo: "DK-99", SessionID: "SESS-99",
		LatestStatus: doku.TransactionStatusSuccess,
	}

	paid, err := f.uc.SyncTransferStatus(ctx, "PO-99")
	require.NoError(t, err)
	require.Equal(t, entity.PayoutStatusPaid, paid.Status)
	require.Equal(t, "DK-99", paid.DokuReferenceNo)
	require.Equal(t, "SESS-99", paid.DokuSessionID)

	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-99", LatestStatus: doku.TransactionStatusRefunded}
	got, err := f.uc.SyncRefundStatus(ctx, "DK-99")

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusFailed, got.Status)
	assert.Contains(t, got.LastError, "status 04")
}
