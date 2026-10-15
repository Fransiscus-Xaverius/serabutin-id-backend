package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

// fakeBidRepo is a minimal in-memory BidRepository used to assert
// race-hardening behavior without spinning up Postgres testcontainers.
// AcceptIfPending here mirrors the DB's WHERE status='pending' guard.
type fakeBidRepo struct {
	bids map[uint]*entity.Bid
	// Track which transition method was called — race-safety requires
	// AcceptIfPending, not the unconditional UpdateStatus.
	calledUpdateStatus    uint
	calledAcceptIfPending uint
}

func newFakeBidRepo(bids ...*entity.Bid) *fakeBidRepo {
	m := make(map[uint]*entity.Bid, len(bids))
	for _, b := range bids {
		m[b.ID] = b
	}
	return &fakeBidRepo{bids: m}
}

func (f *fakeBidRepo) Create(ctx context.Context, bid *entity.Bid) error {
	bid.ID = uint(len(f.bids) + 1)
	f.bids[bid.ID] = bid
	return nil
}
func (f *fakeBidRepo) FindByID(ctx context.Context, id uint) (*entity.Bid, error) {
	b, ok := f.bids[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "BID_NOT_FOUND", "")
	}
	return b, nil
}
func (f *fakeBidRepo) FindByServiceID(ctx context.Context, serviceID uint) ([]*entity.Bid, error) {
	var out []*entity.Bid
	for _, b := range f.bids {
		if b.ServiceID == serviceID {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBidRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.BidFilter) ([]*entity.Bid, int64, error) {
	return nil, 0, nil
}
func (f *fakeBidRepo) CountByStatus(ctx context.Context, providerID uint) (map[string]int64, error) {
	counts := map[string]int64{"pending": 0, "accepted": 0, "rejected": 0}
	for _, b := range f.bids {
		if b.ProviderID == providerID {
			counts[b.Status]++
		}
	}
	return counts, nil
}
func (f *fakeBidRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	f.calledUpdateStatus++
	b, ok := f.bids[id]
	if !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "BID_NOT_FOUND", "")
	}
	b.Status = status
	return nil
}
func (f *fakeBidRepo) AcceptIfPending(ctx context.Context, id uint) error {
	f.calledAcceptIfPending++
	b, ok := f.bids[id]
	if !ok || b.Status != "pending" {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "")
	}
	b.Status = "accepted"
	return nil
}
func (f *fakeBidRepo) RejectOthers(ctx context.Context, serviceID uint, exceptID uint) error {
	for id, b := range f.bids {
		if b.ServiceID == serviceID && id != exceptID && b.Status == "pending" {
			b.Status = "rejected"
		}
	}
	return nil
}

// fakeBookingRepo captures Create calls.
type fakeBookingRepo struct{ created []*entity.Booking }

func (f *fakeBookingRepo) Create(ctx context.Context, b *entity.Booking) error {
	f.created = append(f.created, b)
	return nil
}
func (f *fakeBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *fakeBookingRepo) UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error {
	return nil
}
func (f *fakeBookingRepo) UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error {
	return nil
}
func (f *fakeBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *fakeBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}

type fakeServiceRepo struct{ svc *entity.Service }

func (f *fakeServiceRepo) Create(ctx context.Context, s *entity.Service) error { return nil }
func (f *fakeServiceRepo) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	return f.svc, nil
}
func (f *fakeServiceRepo) Update(ctx context.Context, s *entity.Service) error { return nil }
func (f *fakeServiceRepo) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (f *fakeServiceRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (f *fakeServiceRepo) SetStatus(ctx context.Context, id uint, status string) error { return nil }
func (f *fakeServiceRepo) ListForAdmin(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}

type fakeUserRepo struct{ user *entity.User }

func (f *fakeUserRepo) Create(ctx context.Context, u *entity.User) error { return nil }
func (f *fakeUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	return f.user, nil
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	return f.user, nil
}
func (f *fakeUserRepo) FindByWhatsApp(ctx context.Context, w string) (*entity.User, error) {
	return f.user, nil
}
func (f *fakeUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f *fakeUserRepo) Update(ctx context.Context, u *entity.User) error { return nil }
func (f *fakeUserRepo) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUserRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *fakeUserRepo) UpdateRole(ctx context.Context, id uint, role string) error { return nil }
func (f *fakeUserRepo) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	return nil
}
func (f *fakeUserRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	return 2, nil
}

// TestCreateBid_AutoAcceptUsesRaceSafeTransition asserts that the
// auto-accept path uses AcceptIfPending (conditional), never the
// unconditional UpdateStatus — that's what makes concurrent accepts safe.
func TestCreateBid_AutoAcceptUsesRaceSafeTransition(t *testing.T) {
	bidRepo := newFakeBidRepo()
	bookingRepo := &fakeBookingRepo{}
	svc := &entity.Service{ID: 1, ProviderID: 100, Status: "active"}
	min, max := int64(10000), int64(20000)
	uc := &BidUsecase{
		bidRepo:     bidRepo,
		serviceRepo: &fakeServiceRepo{svc: svc},
		bookingRepo: bookingRepo,
		userRepo:    &fakeUserRepo{user: &entity.User{ID: 42, BudgetMin: &min, BudgetMax: &max}},
		platformFee: 1000,
	}

	createdBid, err := uc.CreateBid(context.Background(), 42, dto.CreateBidRequest{
		ServiceID: 1, Price: 10000, Notes: "diagram example",
	})
	if err != nil {
		t.Fatalf("CreateBid: %v", err)
	}
	if createdBid.Status != "accepted" {
		t.Errorf("auto-accepted bid response status = %q, want accepted", createdBid.Status)
	}

	if bidRepo.calledAcceptIfPending == 0 {
		t.Error("auto-accept must call AcceptIfPending (race-safe), got 0 calls")
	}
	if bidRepo.calledUpdateStatus != 0 {
		t.Errorf("auto-accept must NOT call UpdateStatus, got %d calls", bidRepo.calledUpdateStatus)
	}
	if len(bookingRepo.created) != 1 {
		t.Errorf("expected 1 booking created, got %d", len(bookingRepo.created))
	}
}

func TestCreateBid_CustomerAutoAcceptUsesServiceThreshold(t *testing.T) {
	threshold := int64(15000)
	bidRepo := newFakeBidRepo()
	bookingRepo := &fakeBookingRepo{}
	uc := &BidUsecase{
		bidRepo:     bidRepo,
		serviceRepo: &fakeServiceRepo{svc: &entity.Service{ID: 1, ProviderID: 100, Status: "active", ServiceType: "requested", AutoAcceptPrice: &threshold}},
		bookingRepo: bookingRepo,
		userRepo:    &fakeUserRepo{user: &entity.User{ID: 42}},
		platformFee: 1000,
	}

	createdBid, err := uc.CreateBid(context.Background(), 42, dto.CreateBidRequest{ServiceID: 1, Price: 15000})
	if err != nil {
		t.Fatalf("CreateBid: %v", err)
	}
	if createdBid.Status != "accepted" {
		t.Errorf("customer auto-accepted bid response status = %q, want accepted", createdBid.Status)
	}
	if bidRepo.calledAcceptIfPending != 1 {
		t.Fatalf("customer auto-accept must use AcceptIfPending once, got %d", bidRepo.calledAcceptIfPending)
	}
	if len(bookingRepo.created) != 1 {
		t.Fatalf("expected one booking from customer auto-accept, got %d", len(bookingRepo.created))
	}
}

// TestCompleteAccept_LostRaceSkipsBooking asserts that when the
// race-safe transition returns ErrInvalidTransition (someone else
// already accepted), completeAccept silently skips booking creation
// instead of duplicating it.
func TestCompleteAccept_LostRaceSkipsBooking(t *testing.T) {
	// Pre-seed a bid that is already accepted — AcceptIfPending will fail.
	bid := &entity.Bid{ID: 7, ServiceID: 1, ProviderID: 42, Price: 10000, Status: "accepted"}
	bidRepo := newFakeBidRepo(bid)
	bookingRepo := &fakeBookingRepo{}
	svc := &entity.Service{ID: 1, ProviderID: 100, Status: "active"}
	uc := &BidUsecase{
		bidRepo:     bidRepo,
		serviceRepo: &fakeServiceRepo{svc: svc},
		bookingRepo: bookingRepo,
		userRepo:    &fakeUserRepo{},
		platformFee: 1000,
	}

	// Direct call to completeAccept simulates the loser of the race.
	if _, err := uc.completeAccept(context.Background(), bid, svc); err != nil {
		t.Fatalf("completeAccept on lost race should not error, got %v", err)
	}
	if len(bookingRepo.created) != 0 {
		t.Errorf("lost race must not create a booking, got %d", len(bookingRepo.created))
	}
}

// TestAcceptBid_PropagatesInvalidTransition asserts the manual accept
// path surfaces the race-loss to the caller as an error (unlike
// completeAccept which swallows it for auto-accept).
func TestAcceptBid_PropagatesInvalidTransition(t *testing.T) {
	bid := &entity.Bid{ID: 7, ServiceID: 1, ProviderID: 42, Price: 10000, Status: "rejected"}
	bidRepo := newFakeBidRepo(bid)
	bookingRepo := &fakeBookingRepo{}
	svc := &entity.Service{ID: 1, ProviderID: 100, Status: "active"}
	uc := &BidUsecase{
		bidRepo:     bidRepo,
		serviceRepo: &fakeServiceRepo{svc: svc},
		bookingRepo: bookingRepo,
		userRepo:    &fakeUserRepo{},
		platformFee: 1000,
	}

	_, err := uc.AcceptBid(context.Background(), 100, 7)
	if err == nil {
		t.Fatal("expected INVALID_TRANSITION error on already-rejected bid")
	}
	if !errors.Is(err, apperror.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// keep unused imports happy if test body changes
var _ = time.Now
