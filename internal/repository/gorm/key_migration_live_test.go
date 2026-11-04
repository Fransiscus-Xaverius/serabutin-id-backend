package gormrepo

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// TestLiveKeyMigration_NullableKeysSurvivePopulatedTables rebuilds bookings,
// payments and payouts as they looked before their idempotency key columns
// existed, with rows in each, and then migrates. The key must stay NULL for
// the rows that already exist, and keyless rows written afterwards must
// coexist on the unique index. NULL is distinct for a unique index; an empty
// string would collide on the second row.
func TestLiveKeyMigration_NullableKeysSurvivePopulatedTables(t *testing.T) {
	db := setupBountyLiveDB(t)
	require.NoError(t, db.AutoMigrate(&model.PayoutModel{}))

	require.NoError(t, db.Migrator().DropColumn(&model.BookingModel{}, "idempotency_key"))
	require.NoError(t, db.Migrator().DropColumn(&model.PaymentModel{}, "idempotency_key"))
	require.NoError(t, db.Migrator().DropColumn(&model.PayoutModel{}, "partner_reference_no"))
	require.NoError(t, db.Migrator().DropColumn(&model.PayoutModel{}, "idempotency_key"))
	require.NoError(t, db.Migrator().DropColumn(&model.PayoutModel{}, "external_id"))

	bookingIDs := insertLegacyBookings(t, db, 2)
	for i, bookingID := range bookingIDs {
		require.NoError(t, db.Exec(
			`INSERT INTO payments (booking_id, invoice_number, amount, channel, status, provider)
			 VALUES (?, ?, 155000, 'qris', 'pending', 'doku')`,
			bookingID, fmt.Sprintf("SRB-LEGACY-%d", i),
		).Error)
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Exec(
			`INSERT INTO payouts (booking_id, provider_id, amount, channel, status)
			 VALUES (?, 7, 150000, 'BI_FAST', 'pending')`,
			uint(800+i),
		).Error)
	}

	require.NoError(t, db.AutoMigrate(&model.BookingModel{}, &model.PaymentModel{}, &model.PayoutModel{}))

	assertKeysUniqueAndNull(t, db, "bookings", []string{"idempotency_key"}, 2)
	assertKeysUniqueAndNull(t, db, "payments", []string{"idempotency_key"}, 2)
	assertKeysUniqueAndNull(t, db, "payouts", []string{"partner_reference_no", "idempotency_key", "external_id"}, 2)

	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&model.BookingModel{
			CustomerID: 10, ServiceID: 11, ProviderID: 20,
			SubTotal: 155000, PlatformFee: 5000, Total: 160000,
			PaymentMethod: "qris", Status: "pending_payment",
		}).Error, "a keyless booking must be storable after the migration")

		booking := &model.BookingModel{
			CustomerID: 10, ServiceID: 11, ProviderID: 20,
			SubTotal: 155000, PlatformFee: 5000, Total: 160000,
			PaymentMethod: "qris", Status: "pending_payment",
		}
		require.NoError(t, db.Create(booking).Error)
		require.NoError(t, db.Create(&model.PaymentModel{
			BookingID: booking.ID, InvoiceNumber: fmt.Sprintf("SRB-NEW-%d", i),
			Amount: 155000, Channel: "qris", Status: "pending", Provider: "doku",
		}).Error, "a keyless payment must be storable after the migration")

		require.NoError(t, db.Create(&model.PayoutModel{
			BookingID: uint(900 + i), ProviderID: 7, Amount: 150000,
			Channel: "BI_FAST", Status: "pending",
		}).Error, "a keyless payout must be storable after the migration")
	}

	// Each payment above brings its own booking: 2 legacy + 2 + 2 bookings.
	assertKeysUniqueAndNull(t, db, "bookings", []string{"idempotency_key"}, 6)
	assertKeysUniqueAndNull(t, db, "payments", []string{"idempotency_key"}, 4)
	assertKeysUniqueAndNull(t, db, "payouts", []string{"partner_reference_no", "idempotency_key", "external_id"}, 4)
}

// insertLegacyBookings writes bookings in the pre-key shape and returns their ids.
func insertLegacyBookings(t *testing.T, db *gorm.DB, count int) []uint {
	t.Helper()
	ids := make([]uint, 0, count)
	for i := 0; i < count; i++ {
		var id uint
		require.NoError(t, db.Raw(
			`INSERT INTO bookings (customer_id, service_id, provider_id, sub_total, platform_fee, total, payment_method, status)
			 VALUES (10, 11, 20, 155000, 5000, 160000, 'qris', 'pending_payment') RETURNING id`,
		).Row().Scan(&id))
		ids = append(ids, id)
	}
	return ids
}

// assertKeysUniqueAndNull checks that every key column of a table is unique
// again after the migration, and that exactly nulls rows hold NULL in it. The
// rows that existed before the migration, and the keyless rows written after
// it, must all read NULL.
func assertKeysUniqueAndNull(t *testing.T, db *gorm.DB, table string, columns []string, nulls int64) {
	t.Helper()
	for _, column := range columns {
		var indexDefs []string
		require.NoError(t, db.Raw(
			`SELECT indexdef FROM pg_indexes WHERE tablename = ? AND indexdef LIKE ?`,
			table, "%("+column+")%",
		).Pluck("indexdef", &indexDefs).Error)
		require.NotEmpty(t, indexDefs, "%s.%s must have an index after the migration", table, column)
		assert.Contains(t, indexDefs[0], "UNIQUE", "%s.%s must be unique", table, column)

		var count int64
		require.NoError(t, db.Table(table).Where(column+" IS NULL").Count(&count).Error)
		assert.Equal(t, nulls, count, "%s.%s must be NULL for every keyless row", table, column)
	}
}
