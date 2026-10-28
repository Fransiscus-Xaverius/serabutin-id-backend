package gormrepo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// Live-DB regression tests (F5/F6/F7) against testcontainers Postgres:
// pagination clamps, atomic force-cancel, and broadcast requeue/claim
// recovery. Fakes enforce none of this, so the real repositories are
// exercised here.

func setupBountyLiveDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(
		&model.BookingModel{}, &model.BookingAddOnModel{}, &model.PaymentModel{}, &model.CancellationModel{},
		&model.ServiceModel{}, &model.BroadcastModel{}, &model.PayoutAccountModel{},
	))
	return db
}

func TestLive_ListForAdmin_ClampsNegativePage(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	bookingRepo := NewBookingRepository(db)
	serviceRepo := NewServiceRepository(db)

	// F6: page<1 must behave as page 1 (never a negative OFFSET), and
	// limit>50 must fall back to 20 like the user directory.
	bookings, total, err := bookingRepo.ListForAdmin(ctx, repository.AdminBookingFilter{Page: -3, Limit: 500})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, bookings)

	services, stotal, err := serviceRepo.ListForAdmin(ctx, repository.ServiceFilter{Page: 0, Limit: -5})
	require.NoError(t, err)
	assert.Equal(t, int64(0), stotal)
	assert.Empty(t, services)
}

func TestLive_CreateAndApplyCancel_IsAtomic(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	bookingRepo := NewBookingRepository(db)
	cancelRepo := NewCancellationRepository(db)

	bm := &model.BookingModel{CustomerID: 10, ProviderID: 20, ServiceID: 11, Status: "confirmed", SubTotal: 75000, PlatformFee: 5000, Total: 80000, PaymentMethod: "cash"}
	require.NoError(t, db.WithContext(ctx).Create(bm).Error)

	c := &entity.Cancellation{
		BookingID: bm.ID, CancelledBy: 7, CancelledByRole: "admin",
		Reason: "admin_override:sengketa", PolicyVersion: "admin-override",
	}
	require.NoError(t, cancelRepo.CreateAndApplyCancel(ctx, c, 7))
	require.NotZero(t, c.ID)

	got, err := bookingRepo.FindByID(ctx, bm.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", got.Status)
	require.NotNil(t, got.CancellationID)
	assert.Equal(t, c.ID, *got.CancellationID)

	// Second attempt on the now-terminal booking must fail the guard and
	// leave exactly one cancellation row behind.
	c2 := &entity.Cancellation{
		BookingID: bm.ID, CancelledBy: 7, CancelledByRole: "admin",
		Reason: "admin_override:duplikat", PolicyVersion: "admin-override",
	}
	err = cancelRepo.CreateAndApplyCancel(ctx, c2, 7)
	require.Error(t, err)
	var n int64
	require.NoError(t, db.WithContext(ctx).Model(&model.CancellationModel{}).Where("booking_id = ?", bm.ID).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

func TestLive_Broadcast_RequeueAndStuckClaim(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewBroadcastRepository(db)

	// Simulate an instant blast that died mid-fan-out: sending, no
	// scheduled_at, stale updated_at.
	stale := time.Now().Add(-time.Hour)
	require.NoError(t, db.WithContext(ctx).Exec(
		`INSERT INTO broadcasts (title, body, audience, status, scheduled_at, recipient_count, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		"Promo", "Diskon.", "all", entity.BroadcastStatusSending, nil, 0, 7, stale, stale).Error)

	// F5: the stuck row is reclaimed by ClaimDue.
	due, err := repo.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)

	// F5: Requeue makes an instant failure due immediately (scheduled_at set).
	require.NoError(t, repo.Requeue(ctx, due[0].ID))
	var m model.BroadcastModel
	require.NoError(t, db.WithContext(ctx).First(&m, due[0].ID).Error)
	assert.Equal(t, entity.BroadcastStatusScheduled, m.Status)
	require.NotNil(t, m.ScheduledAt)

	due2, err := repo.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, due2, 1)
}
