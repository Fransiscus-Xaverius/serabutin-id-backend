package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Ticket 5: admin bookings ops — force-cancel any non-terminal booking with
// an admin-override cancellation row, zero fee, and both-party notification.

type adminBookingFakeRepo struct {
	bookings map[uint]*entity.Booking
	listed   repository.AdminBookingFilter
	listErr  error
}

func (f *adminBookingFakeRepo) Create(ctx context.Context, b *entity.Booking) error { return nil }
func (f *adminBookingFakeRepo) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return b, nil
}
func (f *adminBookingFakeRepo) UpdateStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *adminBookingFakeRepo) UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error {
	return nil
}
func (f *adminBookingFakeRepo) UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error {
	b := f.bookings[id]
	b.Status = "cancelled"
	b.CancellationID = &cancellationID
	b.CancelledBy = &cancelledBy
	return nil
}
func (f *adminBookingFakeRepo) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	return nil, 0, nil
}
func (f *adminBookingFakeRepo) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	return nil, nil
}
func (f *adminBookingFakeRepo) ListForAdmin(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	f.listed = filter
	var out []*entity.Booking
	for _, b := range f.bookings {
		out = append(out, b)
	}
	return out, int64(len(out)), nil
}

type adminBookingFakeCancelRepo struct {
	created []*entity.Cancellation
	txErr   error
}

func (f *adminBookingFakeCancelRepo) Create(ctx context.Context, c *entity.Cancellation) error {
	c.ID = uint(len(f.created) + 1)
	f.created = append(f.created, c)
	return nil
}
func (f *adminBookingFakeCancelRepo) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Cancellation, error) {
	return nil, nil
}
func (f *adminBookingFakeCancelRepo) List(ctx context.Context, filter repository.CancellationFilter) ([]*entity.Cancellation, int64, error) {
	return nil, 0, nil
}
func (f *adminBookingFakeCancelRepo) CreateAndApplyCancel(ctx context.Context, c *entity.Cancellation, cancelledBy uint) error {
	if f.txErr != nil {
		return f.txErr
	}
	return f.Create(ctx, c)
}

type adminBookingFakeNotifier struct{ calls []uint }

func (f *adminBookingFakeNotifier) Notify(ctx context.Context, userID uint, typ, title, body, data string) (*entity.Notification, error) {
	f.calls = append(f.calls, userID)
	return &entity.Notification{}, nil
}

func newAdminBookingTestUC(status string) (*AdminBookingUsecase, *adminBookingFakeRepo, *adminBookingFakeCancelRepo, *adminBookingFakeNotifier) {
	bookingRepo := &adminBookingFakeRepo{bookings: map[uint]*entity.Booking{
		1: {ID: 1, CustomerID: 10, ProviderID: 20, ServiceID: 11, Status: status, SubTotal: 75000, PlatformFee: 5000, Total: 80000},
	}}
	cancelRepo := &adminBookingFakeCancelRepo{}
	notifier := &adminBookingFakeNotifier{}
	uc := NewAdminBookingUsecase(bookingRepo, cancelRepo)
	uc.SetNotifier(notifier)
	return uc, bookingRepo, cancelRepo, notifier
}

func TestAdminCancelBooking_PendingWritesOverrideRow(t *testing.T) {
	uc, _, cancelRepo, notifier := newAdminBookingTestUC("pending")

	booking, cancellation, err := uc.AdminCancelBooking(context.Background(), 7, 1, "penipuan terkonfirmasi")
	if err != nil {
		t.Fatalf("AdminCancelBooking: %v", err)
	}
	if booking.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", booking.Status)
	}
	if cancellation.CancelledByRole != "admin" {
		t.Errorf("role = %q, want admin", cancellation.CancelledByRole)
	}
	if cancellation.CancelledBy != 7 {
		t.Errorf("cancelled_by = %d, want 7", cancellation.CancelledBy)
	}
	if cancellation.Reason != "admin_override:penipuan terkonfirmasi" {
		t.Errorf("reason = %q, want admin_override prefix", cancellation.Reason)
	}
	if cancellation.FeeCharged != 0 {
		t.Errorf("fee = %d, want 0", cancellation.FeeCharged)
	}
	if cancellation.PolicyVersion != AdminCancellationPolicyVersion {
		t.Errorf("policy = %q, want %q", cancellation.PolicyVersion, AdminCancellationPolicyVersion)
	}
	if cancellation.PlatformFeeSnapshot != 5000 || cancellation.SubtotalSnapshot != 75000 {
		t.Errorf("snapshots not copied: %+v", cancellation)
	}
	if booking.CancellationID == nil || *booking.CancellationID != cancellation.ID {
		t.Errorf("booking.CancellationID not linked")
	}
	if len(cancelRepo.created) != 1 {
		t.Errorf("created rows = %d, want 1", len(cancelRepo.created))
	}
	if len(notifier.calls) != 2 {
		t.Errorf("notify calls = %d, want 2 (both parties)", len(notifier.calls))
	}
}

func TestAdminCancelBooking_AcceptedAndConfirmedCancellable(t *testing.T) {
	for _, status := range []string{"accepted", "confirmed"} {
		uc, _, _, _ := newAdminBookingTestUC(status)
		booking, _, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa")
		if err != nil {
			t.Errorf("status %s: AdminCancelBooking: %v", status, err)
			continue
		}
		if booking.Status != "cancelled" {
			t.Errorf("status %s: got %q, want cancelled", status, booking.Status)
		}
	}
}

func TestAdminCancelBooking_RejectsTerminal(t *testing.T) {
	for _, status := range []string{"completed", "cancelled", "rejected"} {
		uc, _, cancelRepo, _ := newAdminBookingTestUC(status)
		if _, _, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa"); err == nil {
			t.Errorf("status %s: expected error", status)
		}
		if len(cancelRepo.created) != 0 {
			t.Errorf("status %s: must not write cancellation row", status)
		}
	}
}

func TestAdminCancelBooking_NotFound(t *testing.T) {
	uc, _, _, _ := newAdminBookingTestUC("pending")
	if _, _, err := uc.AdminCancelBooking(context.Background(), 7, 99, "sengketa"); err == nil {
		t.Error("expected error for missing booking")
	}
}

func TestAdminCancelBooking_MissingCancelRepo(t *testing.T) {
	uc := NewAdminBookingUsecase(&adminBookingFakeRepo{bookings: map[uint]*entity.Booking{
		1: {ID: 1, CustomerID: 10, ProviderID: 20, Status: "pending"},
	}}, nil)
	if _, _, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa"); err == nil {
		t.Error("expected error when cancel repo not configured")
	}
}

func TestAdminBookingUsecase_ListBookings_ForwardsFilter(t *testing.T) {
	uc, bookingRepo, _, _ := newAdminBookingTestUC("pending")
	filter := repository.AdminBookingFilter{
		Status: "pending", CustomerID: 10, ProviderID: 20,
		DateFrom: "2026-10-01", DateTo: "2026-10-31", Page: 2, Limit: 10,
	}

	bookings, total, err := uc.ListBookings(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListBookings: %v", err)
	}
	if total != 1 || len(bookings) != 1 {
		t.Errorf("got %d bookings total %d, want 1/1", len(bookings), total)
	}
	if bookingRepo.listed != filter {
		t.Errorf("filter not forwarded: %+v", bookingRepo.listed)
	}
}
