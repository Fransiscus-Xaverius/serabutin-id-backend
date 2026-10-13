package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

// Spec 011 / F1: bookings created from an accepted bid must not carry
// fabricated schedule data; the provider fills it when accepting.
func TestAutoAccept_BookingHasNoFabricatedSchedule(t *testing.T) {
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

	if _, err := uc.CreateBid(context.Background(), 42, dto.CreateBidRequest{ServiceID: 1, Price: 15000}); err != nil {
		t.Fatalf("CreateBid: %v", err)
	}
	if len(bookingRepo.created) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(bookingRepo.created))
	}

	booking := bookingRepo.created[0]
	if booking.ScheduleDate != nil {
		t.Errorf("ScheduleDate = %v, want nil (no fabricated data)", booking.ScheduleDate)
	}
	if booking.ScheduleTime != nil {
		t.Errorf("ScheduleTime = %v, want nil", *booking.ScheduleTime)
	}
	if booking.Address != nil {
		t.Errorf("Address = %v, want nil", *booking.Address)
	}
	if booking.Status != "pending" {
		t.Errorf("Status = %q, want pending", booking.Status)
	}
}

func newAcceptTestUC(status string, withSchedule bool) (*BookingUsecase, *cancelFakeBookingRepo) {
	booking := &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20, Status: status,
		SubTotal: 50000, PlatformFee: 5000, Total: 55000,
	}
	if withSchedule {
		date := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
		timeStr, addr := "09:00", "Jl. Merdeka No. 12"
		booking.ScheduleDate = &date
		booking.ScheduleTime = &timeStr
		booking.Address = &addr
	}
	repo := &cancelFakeBookingRepo{bookings: map[uint]*entity.Booking{1: booking}}
	uc := NewBookingUsecase(repo, nil, nil, 5000, 5000, 7000, nil)
	return uc, repo
}

// Spec 011 / F1 + T-F1.2: bid-derived booking without schedule rejects
// accept when the provider does not supply one.
func TestAcceptBooking_BidDerivedRequiresSchedule(t *testing.T) {
	uc, repo := newAcceptTestUC("pending", false)

	_, err := uc.AcceptBooking(context.Background(), 20, 1, nil)
	if err == nil {
		t.Fatal("expected VALIDATION_ERROR, got nil")
	}
	if repo.bookings[1].Status != "pending" {
		t.Errorf("status = %q, want pending (unchanged)", repo.bookings[1].Status)
	}

	empty := &dto.AcceptBookingRequest{}
	if _, err := uc.AcceptBooking(context.Background(), 20, 1, empty); err == nil {
		t.Fatal("expected VALIDATION_ERROR for empty body, got nil")
	}
}

// Spec 011 / T-F1.3: provider supplies schedule + address → accepted.
func TestAcceptBooking_SetsScheduleAndAccepts(t *testing.T) {
	uc, repo := newAcceptTestUC("pending", false)

	req := &dto.AcceptBookingRequest{
		ScheduleDate: "2026-10-05",
		ScheduleTime: "09:00",
		Address:      "Jl. Merdeka No. 12",
	}
	booking, err := uc.AcceptBooking(context.Background(), 20, 1, req)
	if err != nil {
		t.Fatalf("AcceptBooking: %v", err)
	}
	if booking.Status != "accepted" {
		t.Errorf("status = %q, want accepted", booking.Status)
	}
	saved := repo.bookings[1]
	if saved.ScheduleDate == nil || saved.ScheduleDate.Format("2006-01-02") != "2026-10-05" {
		t.Errorf("ScheduleDate = %v, want 2026-10-05", saved.ScheduleDate)
	}
	if saved.ScheduleTime == nil || *saved.ScheduleTime != "09:00" {
		t.Errorf("ScheduleTime = %v, want 09:00", saved.ScheduleTime)
	}
	if saved.Address == nil || *saved.Address != "Jl. Merdeka No. 12" {
		t.Errorf("Address = %v, want Jl. Merdeka No. 12", saved.Address)
	}
}

// Spec 011 / T-F1.4: direct bookings already carry a schedule, so an
// empty body must keep working.
func TestAcceptBooking_DirectBookingNoBodyStillWorks(t *testing.T) {
	uc, repo := newAcceptTestUC("pending", true)

	booking, err := uc.AcceptBooking(context.Background(), 20, 1, &dto.AcceptBookingRequest{})
	if err != nil {
		t.Fatalf("AcceptBooking: %v", err)
	}
	if booking.Status != "accepted" {
		t.Errorf("status = %q, want accepted", booking.Status)
	}
	if repo.bookings[1].ScheduleDate == nil {
		t.Error("existing schedule must be preserved")
	}
}

func TestAcceptBooking_RejectsInvalidTime(t *testing.T) {
	uc, _ := newAcceptTestUC("pending", false)

	req := &dto.AcceptBookingRequest{
		ScheduleDate: "2026-10-05",
		ScheduleTime: "9am",
		Address:      "Jl. Merdeka No. 12",
	}
	if _, err := uc.AcceptBooking(context.Background(), 20, 1, req); err == nil {
		t.Fatal("expected INVALID_TIME, got nil")
	}
}

// Spec 011 / T-F2.1: counts cover every bid of the provider, not just
// the current page.
func TestGetMyBids_CountsAcrossAllStatuses(t *testing.T) {
	bidRepo := newFakeBidRepo(
		&entity.Bid{ID: 1, ProviderID: 42, Status: "pending"},
		&entity.Bid{ID: 2, ProviderID: 42, Status: "accepted"},
		&entity.Bid{ID: 3, ProviderID: 42, Status: "accepted"},
		&entity.Bid{ID: 4, ProviderID: 42, Status: "rejected"},
		&entity.Bid{ID: 5, ProviderID: 99, Status: "rejected"},
	)
	uc := &BidUsecase{bidRepo: bidRepo}

	_, _, counts, err := uc.GetMyBids(context.Background(), 42, repository.BidFilter{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("GetMyBids: %v", err)
	}
	if counts["pending"] != 1 || counts["accepted"] != 2 || counts["rejected"] != 1 {
		t.Errorf("counts = %v, want pending=1 accepted=2 rejected=1", counts)
	}
}
