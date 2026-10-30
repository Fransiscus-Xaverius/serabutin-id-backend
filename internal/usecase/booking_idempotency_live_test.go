package usecase

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	gormrepo "github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
)

// Live-DB race test: identical concurrent requests with one key go through
// the real repository, so the losing insert hits the real unique index.
func TestLiveCreateBooking_ConcurrentIdenticalRequests_AllSucceedWithOneBooking(t *testing.T) {
	db := setupAdminCancelLiveDB(t)
	ctx := context.Background()
	bookingRepo := gormrepo.NewBookingRepository(db)
	initiator := &raceInitiator{}
	uc := NewBookingUsecase(bookingRepo, &prepayFakeServiceRepo{service: &entity.Service{
		ID: 5, ProviderID: 20, Title: "Bersihkan Rumah", BasePrice: 100000, Status: "active",
		AddOns: []entity.ServiceAddOn{{ID: 1, Name: "Alat sendiri", Price: 25000}},
	}}, nil, 5000, 5000, 7000, &cancelFakeCancelRepo{})
	uc.SetPaymentInitiator(initiator)

	const callers = 8
	bookings := make([]*entity.Booking, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			bookings[i], errs[i] = uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "live-abc")
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(t, errs[i], "caller %d: a lost insert race must not surface as a 500", i)
		assert.Equal(t, bookings[0].ID, bookings[i].ID, "caller %d: all callers get the same booking", i)
	}
	found, err := bookingRepo.FindByIdempotencyKey(ctx, "live-abc")
	require.NoError(t, err)
	assert.Equal(t, bookings[0].ID, found.ID)
	assert.Equal(t, int32(1), initiator.calls.Load(), "only one request starts the checkout")
}
