package gormrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func TestLivePayoutAccount_UpsertAndFind(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPayoutAccountRepository(db)

	account := &entity.PayoutAccount{
		UserID:                     71,
		BankCode:                   "BCAIDJA",
		BankName:                   "Bank Central Asia",
		AccountHolderNameEncrypted: "enc-holder",
		AccountNumberEncrypted:     "enc-number",
		AccountLast4:               "7890",
		AccountFingerprint:         "fp-1",
		EncryptionKeyID:            "v1",
		Status:                     entity.PayoutAccountStatusUnverified,
	}
	require.NoError(t, repo.Upsert(ctx, account))
	assert.NotZero(t, account.ID)

	found, err := repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "BCAIDJA", found.BankCode)
	assert.Equal(t, "7890", found.AccountLast4)

	// Upsert replaces the row for the same user instead of adding one.
	account.BankCode = "MANDIRI"
	account.AccountFingerprint = "fp-2"
	require.NoError(t, repo.Upsert(ctx, account))

	var rows int64
	require.NoError(t, db.Model(&model.PayoutAccountModel{}).Where("user_id = ?", 71).Count(&rows).Error)
	assert.Equal(t, int64(1), rows, "upsert must not leave a second row for the same user")

	found, err = repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "MANDIRI", found.BankCode)
	assert.Equal(t, found.ID, account.ID)
	assert.True(t, account.CreatedAt.Equal(found.CreatedAt), "created_at must reflect the stored row")

	// A fresh struct with no ID and no CreatedAt must come back with the
	// stored id and created_at, not the values Go would have set.
	fresh := &entity.PayoutAccount{
		UserID:                     71,
		BankCode:                   "BNIIDJA",
		BankName:                   "Bank Negara Indonesia",
		AccountHolderNameEncrypted: "enc-holder",
		AccountNumberEncrypted:     "enc-number",
		AccountLast4:               "7890",
		AccountFingerprint:         "fp-3",
		EncryptionKeyID:            "v1",
		Status:                     entity.PayoutAccountStatusUnverified,
	}
	require.NoError(t, repo.Upsert(ctx, fresh))

	stored, err := repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "BNIIDJA", stored.BankCode)
	assert.Equal(t, stored.ID, fresh.ID)
	assert.True(t, fresh.CreatedAt.Equal(stored.CreatedAt), "created_at must come from the stored row")
	assert.Equal(t, found.ID, stored.ID, "the replaced row keeps its primary key")

	byFingerprint, err := repo.FindByFingerprint(ctx, "fp-3")
	require.NoError(t, err)
	assert.Equal(t, uint(71), byFingerprint.UserID)
}

func TestLivePayoutAccount_FindByUserID_Missing(t *testing.T) {
	db := setupBountyLiveDB(t)
	repo := NewPayoutAccountRepository(db)

	_, err := repo.FindByUserID(context.Background(), 999)
	requirePayoutAccountNotFound(t, err)
}

func TestLivePayoutAccount_FindByFingerprint_SelectsByArgument(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPayoutAccountRepository(db)

	for _, a := range []*entity.PayoutAccount{
		{UserID: 72, BankCode: "BCAIDJA", BankName: "Bank Central Asia", AccountHolderNameEncrypted: "enc-holder", AccountNumberEncrypted: "enc-number", AccountLast4: "1111", AccountFingerprint: "fp-a", EncryptionKeyID: "v1", Status: entity.PayoutAccountStatusUnverified},
		{UserID: 73, BankCode: "MANDIRI", BankName: "Bank Mandiri", AccountHolderNameEncrypted: "enc-holder", AccountNumberEncrypted: "enc-number", AccountLast4: "2222", AccountFingerprint: "fp-b", EncryptionKeyID: "v1", Status: entity.PayoutAccountStatusUnverified},
	} {
		require.NoError(t, repo.Upsert(ctx, a))
	}

	second, err := repo.FindByFingerprint(ctx, "fp-b")
	require.NoError(t, err)
	assert.Equal(t, uint(73), second.UserID)
	assert.Equal(t, "MANDIRI", second.BankCode)

	first, err := repo.FindByFingerprint(ctx, "fp-a")
	require.NoError(t, err)
	assert.Equal(t, uint(72), first.UserID)

	_, err = repo.FindByFingerprint(ctx, "fp-unknown")
	requirePayoutAccountNotFound(t, err)
}

func requirePayoutAccountNotFound(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.True(t, errors.As(err, &domainErr), "expected a domain error, got %T", err)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
	assert.Equal(t, "PAYOUT_ACCOUNT_NOT_FOUND", domainErr.Code)
}
