package gormrepo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// TestLivePayoutSchema_NoPlaintextAccountNumber reads the payouts columns from
// the database, not the struct tags. The beneficiary account number must only
// be stored encrypted, so every column that names an account must end in
// _encrypted; the last four digits are the only plain number kept.
func TestLivePayoutSchema_NoPlaintextAccountNumber(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))

	var columns []string
	require.NoError(t, db.Raw(
		`SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'payouts'`,
	).Pluck("column_name", &columns).Error)
	require.NotEmpty(t, columns)

	assert.Contains(t, columns, "beneficiary_account_number_encrypted")
	assert.Contains(t, columns, "beneficiary_last4")
	for _, column := range columns {
		if strings.Contains(column, "account") {
			assert.True(t, strings.HasSuffix(column, "_encrypted"),
				"payouts.%s could hold a plaintext account value; only encrypted account columns are allowed", column)
		}
	}
}
