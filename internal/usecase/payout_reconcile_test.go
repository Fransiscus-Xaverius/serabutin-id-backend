package usecase

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/logger"
)

var (
	errTestDOKUDown    = errors.New("doku unavailable")
	errTestDOKUUnknown = doku.ErrTransactionNotFound
)

// reconcileFixture releases booking 42 (PO-42) and adds two more submitted
// payouts (PO-43, PO-44), so the reconciliation sees three rows, oldest first.
func reconcileFixture(t *testing.T) *releaseFixture {
	t.Helper()
	f, _ := syncFixture(t)
	ctx := context.Background()
	for _, row := range []struct {
		booking uint
		ref     string
	}{{43, "PO-43"}, {44, "PO-44"}} {
		require.NoError(t, f.payouts.Create(ctx, &entity.Payout{
			BookingID: row.booking, ProviderID: 7, Amount: 150000, Currency: "IDR", Channel: payoutChannel,
			Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: row.ref,
		}))
	}
	f.gateway.statusByRef = map[string]*doku.TransactionStatusResult{}
	f.gateway.statusErrByRef = map[string]error{}
	return f
}

// requireStateUnchangedButAttempted is the contract of a failed status check:
// the state and the money fields stay as they were, and the attempt is recorded
// so the next pass waits.
func requireStateUnchangedButAttempted(t *testing.T, before, after entity.Payout) {
	t.Helper()
	assert.Equal(t, before.Status, after.Status)
	assert.Equal(t, before.PaidAt, after.PaidAt)
	assert.Equal(t, before.DokuReferenceNo, after.DokuReferenceNo)
	assert.Equal(t, payoutStatusCheckFailed, after.LastError)
	assert.NotNil(t, after.LastReconciledAt)
}

func statusOf(t *testing.T, f *releaseFixture, ref string) string {
	t.Helper()
	payout, err := f.payouts.FindByPartnerReferenceNo(context.Background(), ref)
	require.NoError(t, err)
	return payout.Status
}

func answer(f *releaseFixture, ref, code string) {
	f.gateway.statusByRef[ref] = &doku.TransactionStatusResult{PartnerReferenceNo: ref, LatestStatus: code}
}

// ageReconciliation makes every row look as if it was last checked long ago,
// so the next pass is due.
func ageReconciliation(f *releaseFixture) {
	past := time.Now().Add(-time.Hour)
	for _, row := range f.payouts.rows {
		row.LastReconciledAt = &past
	}
}

// captureLog routes the payout logs into a buffer at debug level for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := logger.Log
	previousLevel := zerolog.GlobalLevel()
	logger.Log = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		logger.Log = previous
		zerolog.SetGlobalLevel(previousLevel)
	})
	return buf
}

func TestPayoutUsecase_ReconcileSubmitted_ResolvesAPaidTransfer(t *testing.T) {
	f := reconcileFixture(t)
	answer(f, "PO-42", doku.TransactionStatusSuccess)
	answer(f, "PO-43", doku.TransactionStatusPending)
	answer(f, "PO-44", doku.TransactionStatusPending)

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	assert.Equal(t, entity.PayoutStatusPaid, statusOf(t, f, "PO-42"))
	assert.NotNil(t, f.payouts.rows[1].PaidAt)
}

func TestPayoutUsecase_ReconcileSubmitted_LeavesAPendingTransferAlone(t *testing.T) {
	f := reconcileFixture(t)
	answer(f, "PO-42", doku.TransactionStatusPending)
	answer(f, "PO-43", doku.TransactionStatusPending)
	answer(f, "PO-44", doku.TransactionStatusPending)

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-42"))
	assert.Nil(t, f.payouts.rows[1].PaidAt)
}

func TestPayoutUsecase_ReconcileSubmitted_MarksAFailedTransferFailed(t *testing.T) {
	f := reconcileFixture(t)
	answer(f, "PO-42", doku.TransactionStatusFailed)
	answer(f, "PO-43", doku.TransactionStatusPending)
	answer(f, "PO-44", doku.TransactionStatusPending)

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	assert.Equal(t, entity.PayoutStatusFailed, statusOf(t, f, "PO-42"))
}

func TestPayoutUsecase_ReconcileSubmitted_OneFailingRowDoesNotStopTheOthers(t *testing.T) {
	f := reconcileFixture(t)
	f.gateway.statusErrByRef["PO-42"] = errTestDOKUDown
	answer(f, "PO-43", doku.TransactionStatusSuccess)
	answer(f, "PO-44", doku.TransactionStatusFailed)

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err, "a per-row failure is logged and skipped, not returned")
	assert.Equal(t, 2, resolved)
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-42"), "the failing row is left for a later pass")
	assert.Equal(t, entity.PayoutStatusPaid, statusOf(t, f, "PO-43"))
	assert.Equal(t, entity.PayoutStatusFailed, statusOf(t, f, "PO-44"))
}

func TestPayoutUsecase_ReconcileSubmitted_RespectsTheLimitOldestFirst(t *testing.T) {
	f := reconcileFixture(t)

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 2)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Equal(t, []string{"PO-42", "PO-43"}, f.gateway.statuses)
}

func TestPayoutUsecase_ReconcileSubmitted_OnlyAsksAboutSubmittedPayouts(t *testing.T) {
	f := reconcileFixture(t)
	answer(f, "PO-42", doku.TransactionStatusSuccess)
	_, err := f.uc.ReconcileSubmitted(context.Background(), 10)
	require.NoError(t, err)
	ageReconciliation(f)
	f.gateway.statuses = nil

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Equal(t, []string{"PO-43", "PO-44"}, f.gateway.statuses, "the paid PO-42 is never re-checked")
}

func TestPayoutUsecase_ReconcileSubmitted_SecondPassSkipsAnUnknownPayout(t *testing.T) {
	f := reconcileFixture(t)
	f.gateway.statusErrByRef["PO-42"] = errTestDOKUUnknown
	ctx := context.Background()
	captureLog(t)

	_, err := f.uc.ReconcileSubmitted(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"PO-42", "PO-43", "PO-44"}, f.gateway.statuses)

	_, err = f.uc.ReconcileSubmitted(ctx, 10)
	require.NoError(t, err)

	assert.Equal(t, []string{"PO-42", "PO-43", "PO-44"}, f.gateway.statuses, "a payout reconciled less than 5 minutes ago must not reach DOKU again")
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-42"))
	assert.Equal(t, payoutStatusCheckFailed, f.payouts.rows[1].LastError)
}

func TestPayoutUsecase_ReconcileSubmitted_WarnsOnceThenLogsAtDebug(t *testing.T) {
	f, _ := syncFixture(t)
	f.gateway.statusErrByRef = map[string]error{"PO-42": errTestDOKUUnknown}
	ctx := context.Background()
	buf := captureLog(t)

	_, err := f.uc.ReconcileSubmitted(ctx, 10)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"level":"warn"`, "the first unknown result is a warning")
	assert.Contains(t, buf.String(), `"partner_reference_no":"PO-42"`)
	buf.Reset()

	ageReconciliation(f)
	_, err = f.uc.ReconcileSubmitted(ctx, 10)
	require.NoError(t, err)

	assert.Len(t, f.gateway.statuses, 2, "the payout is due again and DOKU is asked again")
	assert.Contains(t, buf.String(), `"level":"debug"`, "a repeat of the same failure logs at debug level")
	assert.NotContains(t, buf.String(), `"level":"warn"`)
}

func TestPayoutUsecase_ReconcileSubmitted_RefusesWithoutReleaseDependencies(t *testing.T) {
	uc := NewPayoutUsecase(nil, nil, nil, "")

	resolved, err := uc.ReconcileSubmitted(context.Background(), 10)

	require.Error(t, err)
	assert.Equal(t, 0, resolved)
}

// A worker started with an already-cancelled context runs its first pass and
// then returns, so the test is deterministic and never sleeps.
func TestPayoutUsecase_ReconcileWorker_RunsOncePassAndStopsOnCancel(t *testing.T) {
	f := reconcileFixture(t)
	answer(f, "PO-42", doku.TransactionStatusSuccess)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		StartPayoutReconcileWorker(ctx, f.uc, time.Hour)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reconcile worker did not stop after its context was cancelled")
	}
	assert.Equal(t, entity.PayoutStatusPaid, statusOf(t, f, "PO-42"))
}

// A deployment without the release configuration runs no pass. The worker
// returns at once and says so once in the log.
func TestPayoutUsecase_ReconcileWorker_DoesNothingWhenTheReleaseIsNotConfigured(t *testing.T) {
	cases := map[string]func(*PayoutRelease){
		"no hub profile":    func(r *PayoutRelease) { r.ProfileID = "" },
		"no source account": func(r *PayoutRelease) { r.FromAccount = "" },
	}
	for name, unset := range cases {
		t.Run(name, func(t *testing.T) {
			f := reconcileFixture(t)
			answer(f, "PO-42", doku.TransactionStatusSuccess)
			f.gateway.inquiries, f.gateway.statuses = nil, nil
			unset(f.uc.release)
			buf := captureLog(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				StartPayoutReconcileWorker(ctx, f.uc, time.Millisecond)
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the reconcile worker kept running without the release configuration")
			}
			assert.Empty(t, f.gateway.statuses)
			assert.Empty(t, f.gateway.inquiries)
			assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-42"))
			assert.Equal(t, 1, strings.Count(buf.String(), "payout reconcile worker not started"), "the refusal is logged once")
		})
	}
}

// The status setter is gone: the only way a payout's status changes is the
// DOKU-backed path. Any new exported method on the usecase must be reviewed
// here before it can be added to this list.
func TestPayoutUsecase_StatusIsOnlySetThroughDOKU(t *testing.T) {
	typ := reflect.TypeOf(&PayoutUsecase{})
	var methods []string
	for i := 0; i < typ.NumMethod(); i++ {
		methods = append(methods, typ.Method(i).Name)
	}
	assert.ElementsMatch(t, []string{
		"Upsert", "Get", "DecryptDestination", "WithRelease",
		"Release", "SyncTransferStatus", "ReconcileSubmitted",
		"ReconcileStalledInquiries",
	}, methods)
}
