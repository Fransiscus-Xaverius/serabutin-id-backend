package gormrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func TestLivePayout_CreateFindAndUniqueness(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	payout := &entity.Payout{
		BookingID:                             42,
		ProviderID:                            7,
		Amount:                                150000,
		Currency:                              "IDR",
		Channel:                               "BCA",
		Status:                                entity.PayoutStatusPending,
		PartnerReferenceNo:                    idempotency.PayoutKey(42),
		RequestFingerprint:                    idempotency.Fingerprint("42|150000|IDR"),
		BeneficiaryBankCode:                   "BCAIDJA",
		BeneficiaryAccountHolderNameEncrypted: "enc-holder",
		BeneficiaryAccountNumberEncrypted:     "enc-number",
		BeneficiaryLast4:                      "7890",
		BeneficiaryFingerprint:                "keyed-fp",
	}
	require.NoError(t, repo.Create(ctx, payout))
	assert.NotZero(t, payout.ID)
	assert.False(t, payout.CreatedAt.IsZero())

	byBooking, err := repo.FindByBookingID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, payout.ID, byBooking.ID)
	assert.Equal(t, "PO-42", byBooking.PartnerReferenceNo)
	assert.Equal(t, "7890", byBooking.BeneficiaryLast4)

	byRef, err := repo.FindByPartnerReferenceNo(ctx, "PO-42")
	require.NoError(t, err)
	assert.Equal(t, payout.ID, byRef.ID)

	byID, err := repo.FindByID(ctx, payout.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(42), byID.BookingID)

	second := &entity.Payout{
		BookingID:          42,
		ProviderID:         7,
		Amount:             150000,
		Currency:           "IDR",
		Channel:            "BCA",
		Status:             entity.PayoutStatusPending,
		PartnerReferenceNo: "PO-other",
	}
	requirePayoutDuplicate(t, repo.Create(ctx, second))

	dupRef := &entity.Payout{
		BookingID:          43,
		ProviderID:         7,
		Amount:             150000,
		Currency:           "IDR",
		Channel:            "BCA",
		Status:             entity.PayoutStatusPending,
		PartnerReferenceNo: "PO-42",
	}
	requirePayoutDuplicate(t, repo.Create(ctx, dupRef))

	var rows int64
	require.NoError(t, db.Model(&model.PayoutModel{}).Where("booking_id = ?", 42).Count(&rows).Error)
	assert.Equal(t, int64(1), rows, "a booking must keep exactly one payout row")
}

func TestLivePayout_EmptyKeysStoreNullAndUpdatePersists(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	first := &entity.Payout{BookingID: 50, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BCA", Status: entity.PayoutStatusPending}
	second := &entity.Payout{BookingID: 51, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BCA", Status: entity.PayoutStatusPending}
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second), "two rows without keys must not collide on the unique indexes")

	var nulls int64
	require.NoError(t, db.Model(&model.PayoutModel{}).Where("partner_reference_no IS NULL AND idempotency_key IS NULL AND external_id IS NULL").Count(&nulls).Error)
	assert.Equal(t, int64(2), nulls)

	found, err := repo.FindByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Empty(t, found.PartnerReferenceNo)

	found.PartnerReferenceNo = idempotency.PayoutKey(found.BookingID)
	found.ExternalID = "9001"
	found.Status = entity.PayoutStatusSubmitted
	require.NoError(t, repo.Update(ctx, found))

	stored, err := repo.FindByPartnerReferenceNo(ctx, "PO-50")
	require.NoError(t, err)
	assert.Equal(t, first.ID, stored.ID)
	assert.Equal(t, "9001", stored.ExternalID)
	assert.Equal(t, entity.PayoutStatusSubmitted, stored.Status)
}

func TestLivePayout_FindMissing(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	_, err := repo.FindByID(ctx, 999)
	requirePayoutNotFound(t, err)
	_, err = repo.FindByBookingID(ctx, 999)
	requirePayoutNotFound(t, err)
	_, err = repo.FindByPartnerReferenceNo(ctx, "PO-999")
	requirePayoutNotFound(t, err)
}

func requirePayoutDuplicate(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.True(t, errors.As(err, &domainErr), "expected a domain error, got %T", err)
	assert.Equal(t, "PAYOUT_DUPLICATE", domainErr.Code)
	assert.ErrorIs(t, err, apperror.ErrAlreadyExists)
}

func requirePayoutNotFound(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.True(t, errors.As(err, &domainErr), "expected a domain error, got %T", err)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
	assert.Equal(t, "PAYOUT_NOT_FOUND", domainErr.Code)
}

func TestLivePayout_FindSubmittedReturnsOldestSubmittedOnly(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	rows := []*entity.Payout{
		{BookingID: 60, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-60"},
		{BookingID: 61, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusPending, PartnerReferenceNo: "PO-61"},
		{BookingID: 62, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-62"},
		{BookingID: 63, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusPaid, PartnerReferenceNo: "PO-63"},
		{BookingID: 64, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-64"},
	}
	for _, row := range rows {
		require.NoError(t, repo.Create(ctx, row))
	}

	submitted, err := repo.FindSubmitted(ctx, 2, time.Now())
	require.NoError(t, err)
	require.Len(t, submitted, 2)
	assert.Equal(t, "PO-60", submitted[0].PartnerReferenceNo, "oldest submitted payout comes first")
	assert.Equal(t, "PO-62", submitted[1].PartnerReferenceNo)

	all, err := repo.FindSubmitted(ctx, 10, time.Now())
	require.NoError(t, err)
	assert.Len(t, all, 3, "pending and paid payouts are never returned")

	none, err := repo.FindSubmitted(ctx, 0, time.Now())
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestLivePayout_FindSubmittedSkipsRecentlyReconciled(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)
	now := time.Now()
	recent := now.Add(-time.Minute)
	old := now.Add(-10 * time.Minute)

	recentRow := &entity.Payout{BookingID: 70, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-70", LastReconciledAt: &recent}
	neverRow := &entity.Payout{BookingID: 71, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-71"}
	oldRow := &entity.Payout{BookingID: 72, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-72", LastReconciledAt: &old}
	for _, row := range []*entity.Payout{recentRow, neverRow, oldRow} {
		require.NoError(t, repo.Create(ctx, row))
	}

	due, err := repo.FindSubmitted(ctx, 10, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.Len(t, due, 2)
	assert.Equal(t, "PO-71", due[0].PartnerReferenceNo, "a never-reconciled payout comes first")
	assert.Equal(t, "PO-72", due[1].PartnerReferenceNo)
}

func TestLivePayout_LastReconciledAtRoundTrips(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	payout := &entity.Payout{BookingID: 73, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-73"}
	require.NoError(t, repo.Create(ctx, payout))
	stamped := time.Now().UTC().Truncate(time.Microsecond)
	payout.LastReconciledAt = &stamped
	require.NoError(t, repo.Update(ctx, payout))

	stored, err := repo.FindByID(ctx, payout.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.LastReconciledAt)
	assert.True(t, stamped.Equal(stored.LastReconciledAt.UTC()))
}

func TestLivePayout_FindStalledInquiryByUpdatedAt(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))
	ctx := context.Background()
	repo := NewPayoutRepository(db)

	stale := &entity.Payout{BookingID: 80, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusInquiry, PartnerReferenceNo: "PO-80"}
	fresh := &entity.Payout{BookingID: 81, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusInquiry, PartnerReferenceNo: "PO-81"}
	submitted := &entity.Payout{BookingID: 82, ProviderID: 7, Amount: 1, Currency: "IDR", Channel: "BI_FAST", Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: "PO-82"}
	for _, row := range []*entity.Payout{stale, fresh, submitted} {
		require.NoError(t, repo.Create(ctx, row))
	}
	require.NoError(t, db.Model(&model.PayoutModel{}).Where("id = ?", stale.ID).UpdateColumn("updated_at", time.Now().Add(-10*time.Minute)).Error)
	require.NoError(t, db.Model(&model.PayoutModel{}).Where("id = ?", submitted.ID).UpdateColumn("updated_at", time.Now().Add(-10*time.Minute)).Error)

	stalled, err := repo.FindStalledInquiry(ctx, 10, time.Now().Add(-5*time.Minute))
	require.NoError(t, err)
	require.Len(t, stalled, 1, "only the inquiry that has sat for more than 5 minutes")
	assert.Equal(t, "PO-80", stalled[0].PartnerReferenceNo)
}
