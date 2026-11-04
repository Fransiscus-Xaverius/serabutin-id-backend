package gormrepo

import (
	"context"
	"errors"
	"testing"

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

	submitted, err := repo.FindSubmitted(ctx, 2)
	require.NoError(t, err)
	require.Len(t, submitted, 2)
	assert.Equal(t, "PO-60", submitted[0].PartnerReferenceNo, "oldest submitted payout comes first")
	assert.Equal(t, "PO-62", submitted[1].PartnerReferenceNo)

	all, err := repo.FindSubmitted(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, all, 3, "pending and paid payouts are never returned")

	none, err := repo.FindSubmitted(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, none)
}
