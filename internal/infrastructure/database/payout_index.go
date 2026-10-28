package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const payoutFingerprintIndex = "idx_payout_accounts_account_fingerprint"

// EnsurePayoutFingerprintIndexUnique makes the payout account fingerprint
// index unique. AutoMigrate creates it unique on a fresh database but never
// changes an index that already exists, so a database created by the earlier
// non-unique model keeps a non-unique index and duplicate bank accounts could
// still be stored. It is idempotent: an index that is already unique is left
// alone. It refuses to run while duplicate fingerprints exist, naming the
// users involved so they can be resolved before the server starts.
func EnsurePayoutFingerprintIndexUnique(db *gorm.DB) error {
	var indexes []struct {
		IndexDef string `gorm:"column:indexdef"`
	}
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'payout_accounts' AND indexname = ?`, payoutFingerprintIndex).Scan(&indexes).Error; err != nil {
		return fmt.Errorf("failed to inspect the payout_accounts indexes: %w", err)
	}
	if len(indexes) > 0 && strings.HasPrefix(indexes[0].IndexDef, "CREATE UNIQUE INDEX") {
		return nil
	}

	var duplicates []struct {
		UserIDs string `gorm:"column:user_ids"`
	}
	if err := db.Raw(`SELECT string_agg(user_id::text, ',' ORDER BY user_id) AS user_ids FROM payout_accounts WHERE account_fingerprint IS NOT NULL GROUP BY account_fingerprint HAVING COUNT(*) > 1 ORDER BY MIN(user_id)`).Scan(&duplicates).Error; err != nil {
		return fmt.Errorf("failed to check payout_accounts for duplicate bank accounts: %w", err)
	}
	if len(duplicates) > 0 {
		groups := make([]string, 0, len(duplicates))
		for _, d := range duplicates {
			groups = append(groups, "["+d.UserIDs+"]")
		}
		return fmt.Errorf("table payout_accounts has %d bank account(s) registered to more than one mitra (user ids %s); resolve the duplicates so each bank account belongs to one mitra, then restart the server", len(duplicates), strings.Join(groups, " "))
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DROP INDEX IF EXISTS ` + payoutFingerprintIndex).Error; err != nil {
			return fmt.Errorf("failed to drop the non-unique payout_accounts fingerprint index: %w", err)
		}
		if err := tx.Exec(`CREATE UNIQUE INDEX ` + payoutFingerprintIndex + ` ON payout_accounts (account_fingerprint)`).Error; err != nil {
			return fmt.Errorf("failed to create the unique payout_accounts fingerprint index: %w", err)
		}
		return nil
	})
}
