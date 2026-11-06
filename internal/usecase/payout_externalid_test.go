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
}

func TestPayoutUsecase_Release_SendsEachStepWithItsOwnExternalID(t *testing.T) {
	f := newReleaseFixture(t)

	payout, err := f.uc.Release(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, uint(1), payout.ID)
	assert.Equal(t, []string{"11"}, f.gateway.inquiryIDs, "inquiry is payoutID*10+1")
	assert.Equal(t, []string{"12"}, f.gateway.paymentIDs, "payment is payoutID*10+2, never the inquiry id")
	assert.Equal(t, []string{"1000000000"}, f.gateway.balanceIDs, "the balance check is a read: the first value of the read sequence")
	assert.Empty(t, f.gateway.statusIDs)
	assert.Equal(t, "12", f.payouts.rows[payout.ID].ExternalID, "the transfer's external id is persisted")
}

// A read-only call never reuses an id: each one takes the payout's next attempt.
func TestPayoutUsecase_SyncTransferStatus_EachReadTakesTheNextAttempt(t *testing.T) {
	f, _ := syncFixture(t)
	require.Equal(t, []string{"1000000000"}, f.gateway.balanceIDs)

	_, err := f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)
	_, err = f.uc.SyncTransferStatus(context.Background(), "PO-42")
	require.NoError(t, err)

	assert.Equal(t, []string{"1000000001", "1000000002"}, f.gateway.statusIDs, "each status read takes the next value of the read sequence")
	assert.Equal(t, []string{"1000000000"}, f.gateway.balanceIDs)
}

// A retry of the same money-moving step reuses its id, so DOKU deduplicates it.
// The read-only calls of the retries each take a new attempt.
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
	assert.Equal(t, []string{"1000000001", "1000000002"}, f.gateway.statusIDs)
	assert.Equal(t, []string{"1000000000", "1000000003"}, f.gateway.balanceIDs)
	assert.Equal(t, "12", f.payouts.rows[payout.ID].ExternalID)
}
