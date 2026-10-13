package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type BookingUsecase struct {
	bookingRepo       repository.BookingRepository
	serviceRepo       repository.ServiceRepository
	userRepo          repository.UserRepository
	cancelRepo        repository.CancellationRepository
	notifier          Notifier
	platformFee       int64
	cancelFeeCustomer int64
	cancelFeeProvider int64
}

func NewBookingUsecase(
	bookingRepo repository.BookingRepository,
	serviceRepo repository.ServiceRepository,
	userRepo repository.UserRepository,
	platformFee int64,
	cancelFeeCustomer int64,
	cancelFeeProvider int64,
	cancelRepo repository.CancellationRepository,
) *BookingUsecase {
	return &BookingUsecase{
		bookingRepo:       bookingRepo,
		serviceRepo:       serviceRepo,
		userRepo:          userRepo,
		cancelRepo:        cancelRepo,
		platformFee:       platformFee,
		cancelFeeCustomer: cancelFeeCustomer,
		cancelFeeProvider: cancelFeeProvider,
	}
}

func (uc *BookingUsecase) SetNotifier(n Notifier) {
	uc.notifier = n
}

// notifyBoth writes one notification row per participant. Best-effort:
// failures are logged and never fail the booking transition.
func (uc *BookingUsecase) notifyBoth(ctx context.Context, booking *entity.Booking, typ, title, body string) {
	if uc.notifier == nil {
		return
	}
	data := fmt.Sprintf(`{"booking_id":%d,"status":%q}`, booking.ID, booking.Status)
	if _, err := uc.notifier.Notify(ctx, booking.CustomerID, typ, title, body, data); err != nil {
		log.Printf("[BookingUsecase.notify] customer %d: %v", booking.CustomerID, err)
	}
	if _, err := uc.notifier.Notify(ctx, booking.ProviderID, typ, title, body, data); err != nil {
		log.Printf("[BookingUsecase.notify] provider %d: %v", booking.ProviderID, err)
	}
}

func (uc *BookingUsecase) CreateBooking(ctx context.Context, customerID uint, req dto.CreateBookingRequest) (*entity.Booking, error) {
	service, err := uc.serviceRepo.FindByID(ctx, req.ServiceID)
	if err != nil {
		log.Printf("[BookingUsecase.CreateBooking] find service error: %v", err)
		return nil, err
	}

	if service.Status != "active" {
		return nil, apperror.NewDomainError(apperror.ErrServiceInactive, "SERVICE_INACTIVE", "service is not active")
	}

	scheduleDate, err := parseScheduleDate(req.ScheduleDate)
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_DATE", "invalid schedule date format, expected YYYY-MM-DD")
	}

	booked, err := uc.bookingRepo.FindBookedSlotsByProvider(ctx, service.ProviderID, req.ScheduleDate)
	if err != nil {
		return nil, err
	}
	for _, taken := range booked {
		if taken == req.ScheduleTime {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "SLOT_UNAVAILABLE", "this time slot is already booked")
		}
	}

	subTotal := service.BasePrice
	var bookingAddOns []entity.BookingAddOn

	if len(req.AddOnIDs) > 0 {
		addOnMap := make(map[uint]entity.ServiceAddOn)
		for _, addon := range service.AddOns {
			addOnMap[addon.ID] = addon
		}

		for _, addOnID := range req.AddOnIDs {
			addon, ok := addOnMap[addOnID]
			if !ok {
				return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_ADDON", "add-on does not belong to this service")
			}
			subTotal += addon.Price
			bookingAddOns = append(bookingAddOns, entity.BookingAddOn{
				Name:  addon.Name,
				Price: addon.Price,
			})
		}
	}

	total := subTotal + uc.platformFee

	booking := &entity.Booking{
		CustomerID:    customerID,
		ServiceID:     service.ID,
		ProviderID:    service.ProviderID,
		ScheduleDate:  &scheduleDate,
		ScheduleTime:  &req.ScheduleTime,
		Address:       &req.Address,
		Notes:         req.Notes,
		SubTotal:      subTotal,
		PlatformFee:   uc.platformFee,
		Total:         total,
		PaymentMethod: req.PaymentMethod,
		Status:        "pending",
		BookingAddOns: bookingAddOns,
	}

	if err := uc.bookingRepo.Create(ctx, booking); err != nil {
		log.Printf("[BookingUsecase.CreateBooking] create error: %v", err)
		return nil, err
	}

	uc.notifyBoth(ctx, booking, "booking.created", "Pesanan baru", "Pesanan baru dibuat dan menunggu konfirmasi mitra.")
	return booking, nil
}

func (uc *BookingUsecase) GetBooking(ctx context.Context, userID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.GetBooking] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != userID && booking.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrNotBookingParticipant, "NOT_PARTICIPANT", "you are not a participant of this booking")
	}

	return booking, nil
}

func (uc *BookingUsecase) GetMyBookings(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	bookings, total, err := uc.bookingRepo.FindByUserID(ctx, userID, role, filter)
	if err != nil {
		log.Printf("[BookingUsecase.GetMyBookings] find error: %v", err)
		return nil, 0, err
	}
	return bookings, total, nil
}

func (uc *BookingUsecase) AcceptBooking(ctx context.Context, providerID uint, id uint, req *dto.AcceptBookingRequest) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.AcceptBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can accept this booking")
	}

	if booking.Status != "pending" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only accept pending bookings")
	}

	// Bookings created from an accepted bid carry no schedule/address (the
	// bid flow has no such data). The provider must supply them here; direct
	// bookings already have them from the customer's checkout form.
	if booking.ScheduleDate == nil || booking.ScheduleTime == nil || booking.Address == nil {
		if req == nil || req.ScheduleDate == "" || req.ScheduleTime == "" || req.Address == "" {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "jadwal dan alamat wajib diisi untuk pesanan ini")
		}

		scheduleDate, err := parseScheduleDate(req.ScheduleDate)
		if err != nil {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_DATE", "invalid schedule date format, expected YYYY-MM-DD")
		}
		if _, err := time.Parse("15:04", req.ScheduleTime); err != nil {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_TIME", "invalid schedule time format, expected HH:MM")
		}

		if err := uc.bookingRepo.UpdateSchedule(ctx, id, scheduleDate, req.ScheduleTime, req.Address); err != nil {
			log.Printf("[BookingUsecase.AcceptBooking] update schedule error: %v", err)
			return nil, err
		}

		booking.ScheduleDate = &scheduleDate
		booking.ScheduleTime = &req.ScheduleTime
		booking.Address = &req.Address
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "accepted"); err != nil {
		log.Printf("[BookingUsecase.AcceptBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "accepted"
	uc.notifyBoth(ctx, booking, "booking.accepted", "Pesanan diterima", "Mitra menerima pesanan Anda.")
	return booking, nil
}

func (uc *BookingUsecase) RejectBooking(ctx context.Context, providerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.RejectBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can reject this booking")
	}

	if booking.Status != "pending" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only reject pending bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "rejected"); err != nil {
		log.Printf("[BookingUsecase.RejectBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "rejected"
	uc.notifyBoth(ctx, booking, "booking.rejected", "Pesanan ditolak", "Mitra menolak pesanan Anda.")
	return booking, nil
}

func (uc *BookingUsecase) ConfirmBooking(ctx context.Context, providerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.ConfirmBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can confirm this booking")
	}

	if booking.Status != "accepted" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only confirm accepted bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "confirmed"); err != nil {
		log.Printf("[BookingUsecase.ConfirmBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "confirmed"
	uc.notifyBoth(ctx, booking, "booking.confirmed", "Pekerjaan dimulai", "Mitra mengkonfirmasi dan mulai mengerjakan pesanan.")
	return booking, nil
}

func (uc *BookingUsecase) CompleteBooking(ctx context.Context, customerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.CompleteBooking] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != customerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the customer can complete this booking")
	}

	if booking.Status != "confirmed" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only complete confirmed bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "completed"); err != nil {
		log.Printf("[BookingUsecase.CompleteBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "completed"
	uc.notifyBoth(ctx, booking, "booking.completed", "Pesanan selesai", "Pesanan ditandai selesai. Silakan beri ulasan.")
	return booking, nil
}

func (uc *BookingUsecase) CancelBooking(ctx context.Context, userID uint, id uint, reason string, detail string) (*entity.Booking, *entity.Cancellation, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.CancelBooking] find error: %v", err)
		return nil, nil, err
	}

	if booking.CustomerID != userID && booking.ProviderID != userID {
		return nil, nil, apperror.NewDomainError(apperror.ErrNotBookingParticipant, "NOT_PARTICIPANT", "you are not a participant of this booking")
	}

	if booking.Status != "pending" && booking.Status != "accepted" && booking.Status != "confirmed" {
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only cancel pending, accepted or confirmed bookings")
	}

	if reason == "" || !entity.ValidCancelReasons[reason] {
		return nil, nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_REASON", "cancel reason is required (changed_mind, found_other, schedule_conflict, no_response, other)")
	}

	role := "customer"
	if userID == booking.ProviderID {
		role = "provider"
	}

	// Grab rule: pending = free; mitra cancel on accepted/confirmed = provider
	// fee; customer cancel on confirmed (in-progress) = customer fee.
	var fee int64
	switch booking.Status {
	case "pending":
		fee = 0
	case "accepted":
		if role == "provider" {
			fee = uc.cancelFeeProvider
		}
	case "confirmed":
		if role == "provider" {
			fee = uc.cancelFeeProvider
		} else {
			fee = uc.cancelFeeCustomer
		}
	}

	cancellation := &entity.Cancellation{
		BookingID:           booking.ID,
		CancelledBy:         userID,
		CancelledByRole:     role,
		Reason:              reason,
		Detail:              detail,
		FeeCharged:          fee,
		PlatformFeeSnapshot: booking.PlatformFee,
		SubtotalSnapshot:    booking.SubTotal,
		PolicyVersion:       entity.CancellationPolicyVersion,
	}
	if uc.cancelRepo == nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInternalServer, "CANCEL_REPO_MISSING", "cancellation repository not configured")
	}
	if err := uc.cancelRepo.Create(ctx, cancellation); err != nil {
		log.Printf("[BookingUsecase.CancelBooking] create cancellation error: %v", err)
		return nil, nil, err
	}

	if err := uc.bookingRepo.UpdateCancel(ctx, id, cancellation.ID, userID); err != nil {
		log.Printf("[BookingUsecase.CancelBooking] update cancel error: %v", err)
		return nil, nil, err
	}

	booking.Status = "cancelled"
	booking.CancellationID = &cancellation.ID
	booking.CancelledBy = &userID
	now := cancellation.CreatedAt
	booking.CancelledAt = &now
	body := "Pesanan dibatalkan tanpa biaya."
	if fee > 0 {
		body = fmt.Sprintf("Pesanan dibatalkan. Biaya pembatalan Rp %d dibebankan ke %s.", fee, role)
	}
	uc.notifyBoth(ctx, booking, "booking.cancelled", "Pesanan dibatalkan", body)
	return booking, cancellation, nil
}

func (uc *BookingUsecase) GetAvailableSlots(ctx context.Context, providerID uint, date string) ([]string, error) {
	booked, err := uc.bookingRepo.FindBookedSlotsByProvider(ctx, providerID, date)
	if err != nil {
		return nil, err
	}

	bookedSet := make(map[string]struct{}, len(booked))
	for _, s := range booked {
		bookedSet[s] = struct{}{}
	}

	allSlots := []string{"08:00", "09:30", "11:00", "13:00", "14:30", "16:00"}
	available := make([]string, 0, len(allSlots))
	for _, slot := range allSlots {
		if _, taken := bookedSet[slot]; !taken {
			available = append(available, slot)
		}
	}
	return available, nil
}

func parseScheduleDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}
