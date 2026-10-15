package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

type cancelFakeBookingRepo struct {
	bookings map[uint]*entity.Booking
}

func (f *cancelFakeBookingRepo) Create(ctx context.Context, b *entity.Booking) error {
	return nil
}
func (f *cancelFakeBookingRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	return f.bookings[id], nil
}
func (f *cancelFakeBookingRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	f.bookings[id].Status = status
	return nil
}
func (f *cancelFakeBookingRepo) UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error {
	b := f.bookings[id]
	b.ScheduleDate = &scheduleDate
	b.ScheduleTime = &scheduleTime
	b.Address = &address
	return nil
}
func (f *cancelFakeBookingRepo) UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error {
	b := f.bookings[id]
	b.Status = "cancelled"
	b.CancellationID = &cancellationID
	b.CancelledBy = &cancelledBy
	return nil
}
func (f *cancelFakeBookingRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *cancelFakeBookingRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *cancelFakeBookingRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}

type cancelFakeCancelRepo struct{ created []*entity.Cancellation }

func (f *cancelFakeCancelRepo) Create(ctx context.Context, c *entity.Cancellation) error {
	c.ID = uint(len(f.created) + 1)
	f.created = append(f.created, c)
	return nil
}
func (f *cancelFakeCancelRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error) {
	return nil, nil
}
func (f *cancelFakeCancelRepo) List(ctx context.Context, filter repository.CancellationFilter) ([]*entity.Cancellation, int64, error) {
	return nil, 0, nil
}
func (f *cancelFakeCancelRepo) CreateAndApplyCancel(ctx context.Context, c *entity.Cancellation, cancelledBy uint) error {
	return f.Create(ctx, c)
}

func newCancelTestUC(status string) (*BookingUsecase, *cancelFakeCancelRepo) {
	bookingRepo := &cancelFakeBookingRepo{bookings: map[uint]*entity.Booking{
		1: {ID: 1, CustomerID: 10, ProviderID: 20, Status: status, SubTotal: 50000, PlatformFee: 5000},
	}}
	cancelRepo := &cancelFakeCancelRepo{}
	uc := NewBookingUsecase(bookingRepo, nil, nil, 5000, 5000, 7000, cancelRepo)
	return uc, cancelRepo
}

func TestCancelBooking_GrabRule(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		status   string
		userID   uint
		wantFee  int64
		wantRole string
	}{
		{"pending customer free", "pending", 10, 0, "customer"},
		{"pending provider free", "pending", 20, 0, "provider"},
		{"accepted provider fee", "accepted", 20, 7000, "provider"},
		{"accepted customer free", "accepted", 10, 0, "customer"},
		{"confirmed customer fee", "confirmed", 10, 5000, "customer"},
		{"confirmed provider fee", "confirmed", 20, 7000, "provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc, _ := newCancelTestUC(tc.status)
			booking, cancellation, err := uc.CancelBooking(ctx, tc.userID, 1, "changed_mind", "")
			if err != nil {
				t.Fatalf("CancelBooking: %v", err)
			}
			if booking.Status != "cancelled" {
				t.Errorf("status = %q, want cancelled", booking.Status)
			}
			if cancellation.FeeCharged != tc.wantFee {
				t.Errorf("fee = %d, want %d", cancellation.FeeCharged, tc.wantFee)
			}
			if cancellation.CancelledByRole != tc.wantRole {
				t.Errorf("role = %q, want %q", cancellation.CancelledByRole, tc.wantRole)
			}
			if booking.CancellationID == nil || *booking.CancellationID != cancellation.ID {
				t.Errorf("booking.CancellationID not linked")
			}
		})
	}
}

func TestCancelBooking_RequiresReason(t *testing.T) {
	uc, _ := newCancelTestUC("pending")
	if _, _, err := uc.CancelBooking(context.Background(), 10, 1, "", ""); err == nil {
		t.Error("expected error for empty reason")
	}
	if _, _, err := uc.CancelBooking(context.Background(), 10, 1, "bogus", ""); err == nil {
		t.Error("expected error for invalid reason")
	}
}

func TestCancelBooking_RejectsTerminal(t *testing.T) {
	uc, _ := newCancelTestUC("completed")
	if _, _, err := uc.CancelBooking(context.Background(), 10, 1, "other", ""); err == nil {
		t.Error("expected error for completed booking")
	}
}
