package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

// inquiryFixture stores a payout stuck in inquiry (PO-45): the crash happened
// after the inquiry was recorded and before any payment was sent. updatedAt
// decides whether it is stale.
func inquiryFixture(t *testing.T, updatedAt time.Time) *releaseFixture {
	t.Helper()
	f := newReleaseFixture(t)
	ctx := context.Background()
	require.NoError(t, f.payouts.Create(ctx, &entity.Payout{
		BookingID: 45, ProviderID: 7, Amount: 150000, Currency: payoutCurrency, Channel: payoutChannel,
		Status: entity.PayoutStatusInquiry, PartnerReferenceNo: "PO-45", DokuInquiryReferenceNo: "INQ-OLD",
		BeneficiaryBankCode: "BCAIDJA", BeneficiaryAccountNumberEncrypted: "enc:1234567890",
		BeneficiaryAccountHolderNameEncrypted: "enc:Siti Rahayu", BeneficiaryLast4: "7890",
		BeneficiaryFingerprint: "keyed-fp",
	}))
	for _, row := range f.payouts.rows {
		if row.PartnerReferenceNo == "PO-45" {
			row.UpdatedAt = updatedAt
		}
	}
	f.gateway.statusByRef = map[string]*doku.TransactionStatusResult{}
	f.gateway.statusErrByRef = map[string]error{}
	return f
}

func TestPayoutUsecase_ReconcileStalledInquiries_RedrivesWithTheSameKey(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = errTestDOKUUnknown

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	require.Len(t, f.gateway.inquiries, 1, "DOKU had no record, so the inquiry is sent again")
	require.Len(t, f.gateway.payments, 1, "and then the payment")
	assert.Equal(t, "PO-45", f.gateway.inquiries[0].PartnerReferenceNo, "the re-drive uses the stored key")
	assert.Equal(t, "PO-45", f.gateway.payments[0].PartnerReferenceNo, "the payment uses the same key, so DOKU deduplicates")
	assert.Equal(t, []string{"13"}, f.gateway.statusIDs, "the status check is asked first, with its own id")
	assert.Equal(t, []string{"11"}, f.gateway.inquiryIDs, "the re-drive inquiry is payoutID*10+1")
	assert.Equal(t, []string{"12"}, f.gateway.paymentIDs, "the re-drive payment is payoutID*10+2")
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-45"))
}

func TestPayoutUsecase_ReconcileStalledInquiries_LeavesAFreshInquiryAlone(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-2*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = errTestDOKUUnknown

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Empty(t, f.gateway.statuses, "a fresh inquiry is not even checked")
	assert.Empty(t, f.gateway.inquiries)
	assert.Empty(t, f.gateway.payments)
	assert.Equal(t, entity.PayoutStatusInquiry, statusOf(t, f, "PO-45"))
}

func TestPayoutUsecase_ReconcileStalledInquiries_AdoptsWhatDOKUAlreadyHolds(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusByRef["PO-45"] = &doku.TransactionStatusResult{PartnerReferenceNo: "PO-45", LatestStatus: doku.TransactionStatusPending}

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	assert.Equal(t, []string{"13"}, f.gateway.statusIDs, "DOKU is asked first")
	assert.Empty(t, f.gateway.inquiries, "DOKU already holds the transfer, so nothing is sent again")
	assert.Empty(t, f.gateway.payments)
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-45"))
}

func TestPayoutUsecase_ReconcileStalledInquiries_FailedRedriveIsNotRetriedOnTheNextPass(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = errTestDOKUUnknown
	f.gateway.inquiryErr = errors.New("doku inquiry down")

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Equal(t, entity.PayoutStatusInquiry, statusOf(t, f, "PO-45"))
	assert.Equal(t, "transfer inquiry failed", f.payouts.rows[1].LastError)

	_, err = f.uc.ReconcileStalledInquiries(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, f.gateway.inquiries, 1, "the failed attempt refreshed updated_at, so the payout waits for the next window")
}

func TestPayoutUsecase_ReconcileStalledInquiries_RespectsTheLimit(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = errTestDOKUUnknown

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 0)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Empty(t, f.gateway.inquiries)
}
