package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// AdminCancellationPolicyVersion tags cancellation rows written by the admin
// force-cancel path (fee override: no fee is ever charged).
const AdminCancellationPolicyVersion = "admin-override"

// AdminBookingUsecase owns admin bookings-ops reads and the dispute
// force-cancel mutation. Single-row writes go through the booking and
// cancellation repositories (the usecase owns the workflow boundary, mirroring
// BookingUsecase.CancelBooking); every mutation is audit-logged by
// middleware.AdminAuditLogger, so the handler enforces a mandatory `reason`
// and this layer never writes audit rows directly.
type AdminBookingUsecase struct {
	bookingRepo repository.BookingRepository
	cancelRepo  repository.CancellationRepository
	notifier    Notifier
}

func NewAdminBookingUsecase(
	bookingRepo repository.BookingRepository,
	cancelRepo repository.CancellationRepository,
) *AdminBookingUsecase {
	return &AdminBookingUsecase{
		bookingRepo: bookingRepo,
		cancelRepo:  cancelRepo,
	}
}

func (uc *AdminBookingUsecase) SetNotifier(n Notifier) {
	uc.notifier = n
}

// ListBookings serves the admin bookings queue (status / customer / provider /
// date-range filters, newest first). Read-only.
func (uc *AdminBookingUsecase) ListBookings(ctx context.Context, filter repository.AdminBookingFilter) ([]*entity.Booking, int64, error) {
	bookings, total, err := uc.bookingRepo.ListForAdmin(ctx, filter)
	if err != nil {
		log.Printf("[AdminBookingUsecase.ListBookings] list bookings error: %v", err)
		return nil, 0, err
	}
	return bookings, total, nil
}

// AdminCancelBooking force-cancels any non-terminal booking as a dispute
// action. Terminal states (completed, cancelled, rejected) are rejected with
// INVALID_TRANSITION (422). The cancellation row carries no fee and the
// admin-override policy version; both parties are notified best-effort.
// F7: the cancellation insert and the booking flip run in one transaction,
// so no crash can leave an orphaned cancellation row. A concurrent terminal
// transition inside the transaction re-reads the booking to keep exact
// 422/404 semantics.
func (uc *AdminBookingUsecase) AdminCancelBooking(ctx context.Context, adminID uint, id uint, reason string) (*entity.Booking, *entity.Cancellation, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminBookingUsecase.AdminCancelBooking] find error: %v", err)
		return nil, nil, err
	}

	switch booking.Status {
	case "completed", "cancelled", "rejected":
		// INVALID_STATUS_TRANSITION (not the legacy INVALID_TRANSITION) so
		// the error maps to 422, following the admin moderation convention.
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "can only cancel non-terminal bookings")
	}

	cancellation := &entity.Cancellation{
		BookingID:           booking.ID,
		CancelledBy:         adminID,
		CancelledByRole:     "admin",
		Reason:              "admin_override:" + reason,
		FeeCharged:          0,
		PlatformFeeSnapshot: booking.PlatformFee,
		SubtotalSnapshot:    booking.SubTotal,
		PolicyVersion:       AdminCancellationPolicyVersion,
	}
	if uc.cancelRepo == nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInternalServer, "CANCEL_REPO_MISSING", "cancellation repository not configured")
	}
	if err := uc.cancelRepo.CreateAndApplyCancel(ctx, cancellation, adminID); err != nil {
		// The guarded write lost a race: re-read to report the true state
		// (404 missing vs 422 terminal) instead of leaking the guard code.
		var domainErr *apperror.DomainError
		if errors.As(err, &domainErr) && domainErr.Code == "CANCEL_GUARD_FAILED" {
			if fresh, ferr := uc.bookingRepo.FindByID(ctx, id); ferr != nil {
				log.Printf("[AdminBookingUsecase.AdminCancelBooking] guard re-read error: %v", ferr)
				return nil, nil, ferr
			} else {
				switch fresh.Status {
				case "completed", "cancelled", "rejected":
					return nil, nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "can only cancel non-terminal bookings")
				}
			}
		}
		log.Printf("[AdminBookingUsecase.AdminCancelBooking] cancel tx error: %v", err)
		return nil, nil, err
	}

	booking.Status = "cancelled"
	booking.CancellationID = &cancellation.ID
	booking.CancelledBy = &adminID
	now := cancellation.CreatedAt
	booking.CancelledAt = &now
	uc.notifyAdminCancel(ctx, booking, reason)
	return booking, cancellation, nil
}

// notifyAdminCancel writes one notification row per participant. Best-effort:
// failures are logged and never fail the admin transition.
func (uc *AdminBookingUsecase) notifyAdminCancel(ctx context.Context, booking *entity.Booking, reason string) {
	if uc.notifier == nil {
		return
	}
	data := fmt.Sprintf(`{"booking_id":%d,"status":%q}`, booking.ID, booking.Status)
	body := fmt.Sprintf("Pesanan dibatalkan oleh admin. Alasan: %s.", reason)
	if _, err := uc.notifier.Notify(ctx, booking.CustomerID, "booking.cancelled", "Pesanan dibatalkan", body, data); err != nil {
		log.Printf("[AdminBookingUsecase.notify] customer %d: %v", booking.CustomerID, err)
	}
	if _, err := uc.notifier.Notify(ctx, booking.ProviderID, "booking.cancelled", "Pesanan dibatalkan", body, data); err != nil {
		log.Printf("[AdminBookingUsecase.notify] provider %d: %v", booking.ProviderID, err)
	}
}
