package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

func TestExternalIDFor_IsPayoutIDTimesTenPlusStep(t *testing.T) {
	payout := &entity.Payout{ID: 123}

	assert.Equal(t, "1231", externalIDFor(payout, externalStepInquiry))
	assert.Equal(t, "1232", externalIDFor(payout, externalStepPayment))
	assert.Equal(t, "1233", externalIDFor(payout, externalStepStatus))
	assert.Equal(t, "1234", externalIDFor(payout, externalStepBalance))
}

func TestPayoutUsecase_Release_SendsEachStepWithItsOwnExternalID(t *testing.T) {
	f := newReleaseFixture(t)

	payout, err := f.uc.Release(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, uint(1), payout.ID)
	assert.Equal(t, []string{"11"}, f.gateway.inquiryIDs, "inquiry is payoutID*10+1")
	assert.Equal(t, []string{"12"}, f.gateway.paymentIDs, "payment is payoutID*10+2, never the inquiry id")
	assert.Equal(t, []string{"14"}, f.gateway.balanceIDs, "the balance check is payoutID*10+4")
	assert.Empty(t, f.gateway.statusIDs)
	assert.Equal(t, "12", f.payouts.rows[payout.ID].ExternalID, "the transfer's external id is persisted")
}

func TestPayoutUsecase_SyncTransferStatus_UsesTheStatusStepID(t *testing.T) {
	f, _ := syncFixture(t)
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusPending}

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")

	require.NoError(t, err)
	assert.Equal(t, []string{"13"}, f.gateway.statusIDs, "status is payoutID*10+3")
}

// A retry of the same step must reuse its id, so DOKU deduplicates it.
func TestPayoutUsecase_Release_RetryOfThePaymentReusesItsExternalID(t *testing.T) {
	f := newReleaseFixture(t)
	ctx := context.Background()
	f.gateway.paymentErr = errors.New("doku payment down")
	_, err := f.uc.Release(ctx, 42)
	require.Error(t, err)

	f.gateway.paymentErr = nil
	f.gateway.statusResult = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-42", LatestStatus: doku.TransactionStatusFailed}
	// The first release after a failure records DOKU's failed answer; the
	// next one sends the transfer again under the same key.
	failed, err := f.uc.Release(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, entity.PayoutStatusFailed, failed.Status)
	payout, err := f.uc.Release(ctx, 42)

	require.NoError(t, err)
	assert.Equal(t, entity.PayoutStatusSubmitted, payout.Status)
	assert.Equal(t, []string{"11", "11"}, f.gateway.inquiryIDs)
	assert.Equal(t, []string{"12", "12"}, f.gateway.paymentIDs)
	assert.Equal(t, "12", f.payouts.rows[payout.ID].ExternalID)
}
