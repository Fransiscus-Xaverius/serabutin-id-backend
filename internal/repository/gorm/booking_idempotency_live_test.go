package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func newKeylessLiveBooking(key string) *entity.Booking {
	return &entity.Booking{
		CustomerID:     10,
		ProviderID:     20,
		ServiceID:      11,
		Status:         entity.BookingStatusPendingPayment,
		SubTotal:       155000,
		PlatformFee:    5000,
		Total:          160000,
		PaymentMethod:  entity.PaymentChannelQRIS,
		IdempotencyKey: key,
	}
}

func TestLiveBooking_MigrationAddsTheKeyToATableThatAlreadyHoldsBookings(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewBookingRepository(db)

	// Rebuild the bookings table as it looked before the idempotency columns
	// existed, with two bookings already in it.
	require.NoError(t, db.Migrator().DropColumn(&model.BookingModel{}, "request_fingerprint"))
	require.NoError(t, db.Migrator().DropColumn(&model.BookingModel{}, "idempotency_key"))
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Exec(
			`INSERT INTO bookings (customer_id, service_id, provider_id, sub_total, platform_fee, total, payment_method, status)
			 VALUES (10, 11, 20, 155000, 5000, 160000, 'qris', 'pending_payment')`,
		).Error)
	}

	// The migration must succeed on a table that already holds bookings.
	require.NoError(t, db.AutoMigrate(&model.BookingModel{}))

	var nulls int64
	require.NoError(t, db.Model(&model.BookingModel{}).
		Where("idempotency_key IS NULL").Count(&nulls).Error)
	assert.Equal(t, int64(2), nulls, "the bookings that existed before the migration keep a NULL key")

	var indexDef string
	require.NoError(t, db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'bookings' AND indexdef LIKE '%idempotency_key%'`,
	).Scan(&indexDef).Error)
	assert.Contains(t, indexDef, "UNIQUE", "the idempotency key must be unique")

	// Keyless bookings written after the migration must all be storable:
	// NULL is distinct for the unique index, '' would not be.
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(ctx, newKeylessLiveBooking("")),
			"a booking without an idempotency key must be storable after the migration")
	}

	keyed := newKeylessLiveBooking("abc")
	keyed.RequestFingerprint = "fingerprint"
	require.NoError(t, repo.Create(ctx, keyed))

	duplicate := newKeylessLiveBooking("abc")
	duplicate.RequestFingerprint = "other"
	require.ErrorIs(t, repo.Create(ctx, duplicate), apperror.ErrDuplicateKey,
		"the unique index must refuse a second booking with the same key as a duplicate-key error")

	found, err := repo.FindByIdempotencyKey(ctx, "abc")
	require.NoError(t, err)
	assert.Equal(t, keyed.ID, found.ID)
	assert.Equal(t, "abc", found.IdempotencyKey)
	assert.Equal(t, "fingerprint", found.RequestFingerprint)

	_, err = repo.FindByIdempotencyKey(ctx, "missing")
	require.ErrorIs(t, err, apperror.ErrBookingNotFound)

	var keyless int64
	require.NoError(t, db.Model(&model.BookingModel{}).
		Where("idempotency_key IS NULL").Count(&keyless).Error)
	assert.Equal(t, int64(5), keyless, "every booking without a key holds NULL, never ''")
}
