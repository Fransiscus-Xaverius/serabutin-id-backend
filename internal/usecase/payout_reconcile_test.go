package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
)

var errTestDOKUDown = errors.New("doku unavailable")

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

func statusOf(t *testing.T, f *releaseFixture, ref string) string {
	t.Helper()
	payout, err := f.payouts.FindByPartnerReferenceNo(context.Background(), ref)
	require.NoError(t, err)
	return payout.Status
}

func answer(f *releaseFixture, ref, code string) {
	f.gateway.statusByRef[ref] = &doku.TransactionStatusResult{PartnerReferenceNo: ref, LatestStatus: code}
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
	assert.Equal(t, entity.PayoutStatusSubmitted, statusOf(t, f, "PO-42"), "the failing row is left for the next pass")
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
	f.gateway.statuses = nil

	resolved, err := f.uc.ReconcileSubmitted(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 0, resolved)
	assert.Equal(t, []string{"PO-43", "PO-44"}, f.gateway.statuses, "the paid PO-42 is never re-checked")
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
	}, methods)
}
