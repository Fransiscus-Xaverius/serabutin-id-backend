package usecase

import (
	"context"
	"errors"
	"fmt"
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

// Only DOKU's typed not-found error means DOKU has no record, so only it may
// re-drive the inquiry and the payment.
func TestPayoutUsecase_ReconcileStalledInquiries_RedrivesWithTheSameKeyOnlyOnNotFound(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = fmt.Errorf("status check: %w", doku.ErrTransactionNotFound)

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	require.Len(t, f.gateway.inquiries, 1, "DOKU has no record, so the inquiry is sent again")
	require.Len(t, f.gateway.payments, 1, "and then the payment")
	assert.Equal(t, "PO-45", f.gateway.inquiries[0].PartnerReferenceNo, "the re-drive uses the stored key")
	assert.Equal(t, "PO-45", f.gateway.payments[0].PartnerReferenceNo, "the payment uses the same key, so DOKU deduplicates")
	assert.Equal(t, []string{"1000000000"}, f.gateway.statusIDs, "the status check is a read: the first value of the read sequence")
	assert.Equal(t, []string{"1000000001"}, f.gateway.balanceIDs, "the settled-balance read takes the next value of the read sequence")
	assert.Equal(t, []string{"11"}, f.gateway.inquiryIDs, "the re-drive inquiry is payoutID*10+1")
	assert.Equal(t, []string{"12"}, f.gateway.paymentIDs, "the re-drive payment is payoutID*10+2")
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-45"))
}

// Every failure other than not-found leaves DOKU's state unknown. The payout
// is skipped for the pass: no inquiry and no payment are sent.
func TestPayoutUsecase_ReconcileStalledInquiries_TransientStatusErrorSendsNothing(t *testing.T) {
	cases := map[string]error{
		"doku unavailable":          errTestDOKUDown,
		"status 503":                errors.New("doku sub-account transactions-status returned HTTP 503"),
		"status 401":                errors.New("doku sub-account transactions-status returned HTTP 401"),
		"status 500":                errors.New("doku sub-account transactions-status returned HTTP 500"),
		"timeout":                   context.DeadlineExceeded,
		"unparseable status answer": errors.New("parse doku sub-account /sub-account/v2.0/transactions-status response"),
	}
	for name, statusErr := range cases {
		t.Run(name, func(t *testing.T) {
			f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
			f.gateway.statusErrByRef["PO-45"] = statusErr

			resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

			require.NoError(t, err, "a per-row failure is logged and skipped, not returned")
			assert.Equal(t, 0, resolved)
			assert.Len(t, f.gateway.statuses, 1, "DOKU was asked once")
			assert.Empty(t, f.gateway.inquiries, "a transient failure must never send an inquiry")
			assert.Empty(t, f.gateway.payments, "a transient failure must never send a payment")
			assert.Equal(t, entity.PayoutStatusInquiry, statusOf(t, f, "PO-45"))
			assert.Equal(t, "transaction status failed", f.payouts.rows[1].LastError)
		})
	}
}

// A transient failure still counts as an attempt: updated_at is refreshed, so
// the next pass waits out the stale window instead of asking DOKU again.
func TestPayoutUsecase_ReconcileStalledInquiries_TransientFailureRefreshesTheTimestamp(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = errTestDOKUDown

	_, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, f.gateway.statuses, 1)

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Len(t, f.gateway.statuses, 1, "the failed attempt refreshed updated_at, so the next pass skips the payout")
	assert.Empty(t, f.gateway.inquiries)
	assert.Empty(t, f.gateway.payments)
}

func TestPayoutUsecase_ReconcileStalledInquiries_LeavesAFreshInquiryAlone(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-2*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = doku.ErrTransactionNotFound

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
	assert.Equal(t, []string{"1000000000"}, f.gateway.statusIDs, "DOKU is asked first")
	assert.Empty(t, f.gateway.inquiries, "DOKU already holds the transfer, so nothing is sent again")
	assert.Empty(t, f.gateway.payments)
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-45"))
}

func TestPayoutUsecase_ReconcileStalledInquiries_FailedRedriveIsNotRetriedOnTheNextPass(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = doku.ErrTransactionNotFound
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

// A stale copy of an inquiry must never move a payout DOKU has since paid.
// The copy is read with status inquiry; the row is already paid.
func TestPayoutUsecase_RedriveWithAStaleCopyCannotMoveAPaidPayoutBack(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = doku.ErrTransactionNotFound
	stale := *f.payouts.rows[1]
	paidAt := time.Now()
	f.payouts.rows[1].Status = entity.PayoutStatusPaid
	f.payouts.rows[1].PaidAt = &paidAt
	release, err := f.uc.releaseDeps()
	require.NoError(t, err)

	moved, err := f.uc.redriveInquiry(context.Background(), release, &stale)

	require.Error(t, err)
	assert.False(t, moved)
	assert.Empty(t, f.gateway.statuses, "the stale copy is refused before DOKU is asked")
	assert.Empty(t, f.gateway.inquiries, "no transfer may be sent on a stale copy")
	assert.Empty(t, f.gateway.payments)
	stored := f.payouts.rows[1]
	assert.Equal(t, entity.PayoutStatusPaid, stored.Status)
	assert.Equal(t, &paidAt, stored.PaidAt)
}

func TestPayoutUsecase_ReconcileStalledInquiries_RespectsTheLimit(t *testing.T) {
	f := inquiryFixture(t, time.Now().Add(-10*time.Minute))
	f.gateway.statusErrByRef["PO-45"] = doku.ErrTransactionNotFound

	resolved, err := f.uc.ReconcileStalledInquiries(context.Background(), 0)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Empty(t, f.gateway.inquiries)
}
