package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func setupPayoutIndexLiveDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := testpostgres.Run(ctx,
		"postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpostgres.Open(connStr), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PayoutAccountModel{}))
	return db
}

// downgradeToNonUniqueFingerprintIndex recreates the index the way the earlier
// non-unique model did: same name, no UNIQUE.
func downgradeToNonUniqueFingerprintIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`DROP INDEX `+payoutFingerprintIndex).Error)
	require.NoError(t, db.Exec(`CREATE INDEX `+payoutFingerprintIndex+` ON payout_accounts (account_fingerprint)`).Error)
}

func payoutFingerprintIndexDef(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var defs []string
	require.NoError(t, db.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = 'payout_accounts' AND indexname = ?`, payoutFingerprintIndex).Pluck("indexdef", &defs).Error)
	require.Len(t, defs, 1)
	return defs[0]
}

func insertPayoutRow(t *testing.T, db *gorm.DB, userID uint, fingerprint string) error {
	t.Helper()
	return db.Create(&model.PayoutAccountModel{
		UserID:                     userID,
		BankCode:                   "BCAIDJA",
		BankName:                   "Bank Central Asia",
		AccountHolderNameEncrypted: "enc-holder",
		AccountNumberEncrypted:     "enc-number",
		AccountLast4:               "7890",
		AccountFingerprint:         fingerprint,
		EncryptionKeyID:            "v1",
		Status:                     "unverified",
	}).Error
}

func TestLivePayoutFingerprintIndex_FreshDatabaseIsNoOp(t *testing.T) {
	db := setupPayoutIndexLiveDB(t)

	require.NoError(t, EnsurePayoutFingerprintIndexUnique(db))
	require.NoError(t, EnsurePayoutFingerprintIndexUnique(db), "a second run must also be a no-op")

	assert.True(t, strings.HasPrefix(payoutFingerprintIndexDef(t, db), "CREATE UNIQUE INDEX"))
}

func TestLivePayoutFingerprintIndex_UpgradesLegacyNonUniqueIndex(t *testing.T) {
	db := setupPayoutIndexLiveDB(t)
	downgradeToNonUniqueFingerprintIndex(t, db)
	require.NoError(t, insertPayoutRow(t, db, 71, "fp-a"))
	require.NoError(t, insertPayoutRow(t, db, 72, "fp-b"))
	require.False(t, strings.HasPrefix(payoutFingerprintIndexDef(t, db), "CREATE UNIQUE INDEX"))

	require.NoError(t, EnsurePayoutFingerprintIndexUnique(db))

	assert.True(t, strings.HasPrefix(payoutFingerprintIndexDef(t, db), "CREATE UNIQUE INDEX"))
	assert.Error(t, insertPayoutRow(t, db, 73, "fp-a"), "the rebuilt index must reject a duplicate fingerprint")
	require.NoError(t, EnsurePayoutFingerprintIndexUnique(db), "running again after the upgrade must be a no-op")
}

func TestLivePayoutFingerprintIndex_RefusesToRebuildOverDuplicates(t *testing.T) {
	db := setupPayoutIndexLiveDB(t)
	downgradeToNonUniqueFingerprintIndex(t, db)
	require.NoError(t, insertPayoutRow(t, db, 71, "fp-shared"))
	require.NoError(t, insertPayoutRow(t, db, 72, "fp-shared"))

	err := EnsurePayoutFingerprintIndexUnique(db)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "payout_accounts")
	assert.Contains(t, err.Error(), "[71,72]")
	assert.NotContains(t, err.Error(), "fp-shared", "the error must not echo the fingerprint")
	assert.False(t, strings.HasPrefix(payoutFingerprintIndexDef(t, db), "CREATE UNIQUE INDEX"), "the index must stay non-unique when the upgrade is refused")
}
