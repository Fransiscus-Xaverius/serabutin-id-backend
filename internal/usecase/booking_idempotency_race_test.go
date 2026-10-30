package usecase

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

// raceBookingRepo is a booking store that is safe for concurrent use and
// enforces the idempotency-key unique index the way the real repository
// does: a second insert of a held key answers ErrDuplicateKey.
type raceBookingRepo struct {
	cancelFakeBookingRepo
	mu sync.Mutex
	// onFirstMiss runs once, after the first lookup by key has answered
	// "not found" and released the lock. A test uses it to let a winning
	// request insert in the gap between this request's lookup and its slot
	// check.
	onFirstMiss func()
	// onFirstFreeSlots runs once, after the first slot lookup has answered
	// with the slots that are free. A test uses it to let a winning request
	// insert in the gap between the slot check and this request's insert.
	onFirstFreeSlots func()
}

func newRaceBookingRepo() *raceBookingRepo {
	return &raceBookingRepo{cancelFakeBookingRepo: cancelFakeBookingRepo{bookings: map[uint]*entity.Booking{}}}
}

func (f *raceBookingRepo) Create(ctx context.Context, b *entity.Booking) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b.IdempotencyKey != "" && f.byKeyLocked(b.IdempotencyKey) != nil {
		return apperror.NewDomainError(apperror.ErrDuplicateKey, "DUPLICATE_KEY", "booking with this idempotency key already exists")
	}
	b.ID = uint(len(f.bookings) + 1)
	f.bookings[b.ID] = b
	return nil
}

func (f *raceBookingRepo) FindByIdempotencyKey(ctx context.Context, key string) (*entity.Booking, error) {
	f.mu.Lock()
	found := f.byKeyLocked(key)
	hook := f.onFirstMiss
	f.onFirstMiss = nil
	f.mu.Unlock()

	if found == nil {
		if hook != nil {
			hook()
		}
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return found, nil
}

// FindBookedSlotsByProvider reports the schedule time of every stored
// booking, like the real repository does for pending_payment rows.
func (f *raceBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	f.mu.Lock()
	var slots []string
	for _, b := range f.bookings {
		if b.ScheduleTime != nil {
			slots = append(slots, *b.ScheduleTime)
		}
	}
	hook := f.onFirstFreeSlots
	f.onFirstFreeSlots = nil
	f.mu.Unlock()

	if hook != nil {
		hook()
	}
	return slots, nil
}

func (f *raceBookingRepo) byKeyLocked(key string) *entity.Booking {
	for _, b := range f.bookings {
		if b.IdempotencyKey == key {
			return b
		}
	}
	return nil
}

func (f *raceBookingRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bookings)
}

// raceWait bounds every wait in these tests, so a broken implementation
// fails the test within the go test timeout instead of hanging it.
const raceWait = 5 * time.Second

// raceInitiator is safe for concurrent use. When started and release are
// set, a checkout signals started and then waits for release (or raceWait),
// which holds a first request inside its checkout. started must be buffered:
// the signal never blocks the checkout.
type raceInitiator struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (f *raceInitiator) CreateCheckout(ctx context.Context, booking *entity.Booking, channel string) (*entity.Payment, error) {
	f.calls.Add(1)
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-time.After(raceWait):
		}
	}
	return &entity.Payment{ID: 1, Status: entity.PaymentStatusPending, Channel: channel}, nil
}

func (f *raceInitiator) RetryCheckout(ctx context.Context, bookingID uint, channel string) (*entity.Payment, error) {
	return nil, nil
}

func (f *raceInitiator) IsPrepaidChannel(channel string) bool {
	return channel == entity.PaymentChannelQRIS
}

func newRaceUC(repo *raceBookingRepo, initiator *raceInitiator) *BookingUsecase {
	serviceRepo := &prepayFakeServiceRepo{service: &entity.Service{
		ID:         5,
		ProviderID: 20,
		Title:      "Bersihkan Rumah",
		BasePrice:  100000,
		Status:     "active",
		AddOns:     []entity.ServiceAddOn{{ID: 1, Name: "Alat sendiri", Price: 25000}},
	}}
	uc := NewBookingUsecase(repo, serviceRepo, nil, 5000, 5000, 7000, &cancelFakeCancelRepo{})
	uc.SetPaymentInitiator(initiator)
	return uc
}

// The winner inserts after this request's slot check said the slot was free
// and before this request's insert: this request loses the unique index and
// must answer with the winner's booking.
func TestCreateBooking_LosesTheUniqueIndexRace_ReturnsTheWinnersBooking(t *testing.T) {
	ctx := context.Background()
	repo := newRaceBookingRepo()
	initiator := &raceInitiator{}
	uc := newRaceUC(repo, initiator)

	var winner *entity.Booking
	repo.onFirstFreeSlots = func() {
		var err error
		winner, err = uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")
		require.NoError(t, err)
	}

	booking, err := uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")

	require.NoError(t, err, "a lost insert race must not surface as an error")
	require.NotNil(t, winner)
	require.NotNil(t, booking)
	assert.Equal(t, winner.ID, booking.ID, "the loser must get the winner's booking")
	assert.Equal(t, 1, repo.count(), "the race must not create a second booking")
	assert.Equal(t, int32(1), initiator.calls.Load(), "only the winner starts a checkout")
}

// The winner inserts after this request's key lookup missed and before its
// slot check: the slot now reads as taken, and the request must still answer
// with the winner's booking rather than SLOT_UNAVAILABLE.
func TestCreateBooking_WinnerTakesTheSlotAfterTheLookup_ReturnsTheWinnersBooking(t *testing.T) {
	ctx := context.Background()
	repo := newRaceBookingRepo()
	initiator := &raceInitiator{}
	uc := newRaceUC(repo, initiator)

	var winner *entity.Booking
	repo.onFirstMiss = func() {
		var err error
		winner, err = uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")
		require.NoError(t, err)
	}

	booking, err := uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")

	require.NoError(t, err, "a slot taken by the same request must not surface as SLOT_UNAVAILABLE")
	require.NotNil(t, winner)
	assert.Equal(t, winner.ID, booking.ID)
	assert.Equal(t, 1, repo.count())
	assert.Equal(t, int32(1), initiator.calls.Load())
}

// A repeat that arrives while the first request is still inside its checkout
// gets the stored booking, with Payment still nil, and starts no checkout of
// its own. The checkout URL is fetched afterwards, see CreateBooking.
func TestCreateBooking_InFlightRepeat_ReturnsTheStoredBookingWithoutPayment(t *testing.T) {
	ctx := context.Background()
	repo := newRaceBookingRepo()
	initiator := &raceInitiator{started: make(chan struct{}, 1), release: make(chan struct{})}
	uc := newRaceUC(repo, initiator)

	type result struct {
		booking *entity.Booking
		err     error
	}
	first := make(chan result, 1)
	go func() {
		booking, err := uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")
		first <- result{booking, err}
	}()
	select {
	case <-initiator.started:
	case <-time.After(raceWait):
		t.Fatal("the first checkout never started")
	}

	repeat, err := uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")

	require.NoError(t, err)
	require.NotNil(t, repeat)
	assert.Nil(t, repeat.Payment, "the repeat answers with the stored booking; its checkout is still in flight")
	assert.Equal(t, int32(1), initiator.calls.Load(), "the repeat must not start a second checkout")

	close(initiator.release)
	var got result
	select {
	case got = <-first:
	case <-time.After(raceWait):
		t.Fatal("the first request never finished")
	}
	require.NoError(t, got.err)
	assert.Equal(t, got.booking.ID, repeat.ID, "both callers end up with the same booking")
	assert.Equal(t, 1, repo.count())
}

func TestCreateBooking_ConcurrentIdenticalRequests_AllSucceedWithOneBooking(t *testing.T) {
	ctx := context.Background()
	repo := newRaceBookingRepo()
	initiator := &raceInitiator{}
	uc := newRaceUC(repo, initiator)

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
			bookings[i], errs[i] = uc.CreateBooking(ctx, 10, prepayRequest(entity.PaymentChannelQRIS), "abc")
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(t, errs[i], "caller %d: identical concurrent requests must all succeed", i)
		assert.Equal(t, bookings[0].ID, bookings[i].ID, "caller %d: all callers get the same booking", i)
	}
	assert.Equal(t, 1, repo.count())
	assert.Equal(t, int32(1), initiator.calls.Load(), "only one request starts the checkout")
}
