package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
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

	found, err = repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "MANDIRI", found.BankCode)
	assert.Equal(t, found.ID, account.ID)
	assert.True(t, account.CreatedAt.Equal(found.CreatedAt), "created_at must reflect the stored row")

	byFingerprint, err := repo.FindByFingerprint(ctx, "fp-2")
	require.NoError(t, err)
	assert.Equal(t, uint(71), byFingerprint.UserID)
}

func TestLivePayoutAccount_FindByUserID_Missing(t *testing.T) {
	db := setupBountyLiveDB(t)
	repo := NewPayoutAccountRepository(db)

	_, err := repo.FindByUserID(context.Background(), 999)
	require.Error(t, err)
}
