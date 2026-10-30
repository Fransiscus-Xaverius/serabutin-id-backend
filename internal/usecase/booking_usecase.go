package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/idempotency"
)

type BookingUsecase struct {
	bookingRepo       repository.BookingRepository
	serviceRepo       repository.ServiceRepository
	userRepo          repository.UserRepository
	cancelRepo        repository.CancellationRepository
	notifier          Notifier
	paymentInitiator  PaymentInitiator
	refundMarker      PaymentRefundMarker
	platformFee       int64
	cancelFeeCustomer int64
	cancelFeeProvider int64
}

// PaymentInitiator creates the provider checkout for a newly created
// booking and knows which payment channels are enabled. Implemented by
// PaymentUsecase; nil means the deployment has no payment gateway wired and
// prepaid bookings cannot be started.
type PaymentInitiator interface {
	CreateCheckout(ctx context.Context, booking *entity.Booking, channel string) (*entity.Payment, error)
	RetryCheckout(ctx context.Context, bookingID uint, channel string) (*entity.Payment, error)
	IsPrepaidChannel(channel string) bool
}

// PaymentRefundMarker records the manual refund obligation for a paid
// booking that was rejected or cancelled before the work was delivered.
type PaymentRefundMarker interface {
	MarkRefundPendingForBooking(ctx context.Context, bookingID uint) error
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

// SetPaymentInitiator wires the provider checkout used by prepaid bookings.
func (uc *BookingUsecase) SetPaymentInitiator(p PaymentInitiator) {
	uc.paymentInitiator = p
}

// SetPaymentRefundMarker wires the refund-obligation recorder.
func (uc *BookingUsecase) SetPaymentRefundMarker(m PaymentRefundMarker) {
	uc.refundMarker = m
}

// notifyCustomer writes one notification row for the customer only.
// Best-effort: failures are logged and never fail the booking transition.
func (uc *BookingUsecase) notifyCustomer(ctx context.Context, booking *entity.Booking, typ, title, body string) {
	if uc.notifier == nil {
		return
	}
	data := fmt.Sprintf(`{"booking_id":%d,"status":%q}`, booking.ID, booking.Status)
	if _, err := uc.notifier.Notify(ctx, booking.CustomerID, typ, title, body, data); err != nil {
		log.Printf("[BookingUsecase.notify] customer %d: %v", booking.CustomerID, err)
	}
}

// markRefundPending records that a paid booking lost its provider and that
// the customer is owed money back. No money moves here: the refund itself is
// handled manually by an operator.
func (uc *BookingUsecase) markRefundPending(ctx context.Context, booking *entity.Booking) {
	if uc.refundMarker == nil || booking == nil {
		return
	}
	if booking.Payment == nil || booking.Payment.Status != entity.PaymentStatusPaid {
		return
	}
	if err := uc.refundMarker.MarkRefundPendingForBooking(ctx, booking.ID); err != nil {
		log.Printf("[BookingUsecase.markRefundPending] booking %d: %v", booking.ID, err)
		return
	}
	booking.Payment.Status = entity.PaymentStatusRefundPending
	uc.notifyCustomer(ctx, booking, "payment.refund_pending",
		"Pembatalan diproses",
		"Pembayaran Anda diproses pengembaliannya. Dana dikembalikan sesuai kebijakan pembatalan, dikurangi biaya penyedia pembayaran bila berlaku.")
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

// CreateBooking creates the booking and starts its checkout. A non-empty
// idempotencyKey is bound to this request: a repeat of the same request by
// the same customer returns the booking that was already created, and a key
// reused for another request or another customer is refused.
//
// A repeat that arrives while the first request is still inside its checkout
// returns the stored booking with no payment yet (Payment is nil). The repeat
// starts no second checkout. The client fetches the checkout afterwards with
// POST /api/v1/bookings/:id/payment/retry, or polls GET
// /api/v1/bookings/:id/payment.
func (uc *BookingUsecase) CreateBooking(ctx context.Context, customerID uint, req dto.CreateBookingRequest, idempotencyKey string) (*entity.Booking, error) {
	fingerprint := ""
	if idempotencyKey != "" {
		fingerprint = idempotency.Fingerprint(idempotencyKey, bookingCanonicalRequest(req))
		existing, err := uc.replayBooking(ctx, customerID, idempotencyKey, fingerprint)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

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
			// A concurrent request with the same key may have just taken the
			// slot with its own booking. That is the same request, so answer
			// with its booking rather than SLOT_UNAVAILABLE.
			if idempotencyKey != "" {
				existing, err := uc.replayBooking(ctx, customerID, idempotencyKey, fingerprint)
				if err != nil {
					return nil, err
				}
				if existing != nil {
					return existing, nil
				}
			}
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

	// Prepaid bookings are created as pending_payment: the Mitra only sees
	// the booking once the payment webhook confirms the money arrived. Every
	// other payment method is refused — an order that is never paid for must
	// not be creatable through the API.
	if uc.paymentInitiator == nil {
		return nil, apperror.NewDomainError(apperror.ErrPaymentNotConfigured, "PAYMENT_NOT_CONFIGURED", "payment is not available right now")
	}
	if !uc.paymentInitiator.IsPrepaidChannel(req.PaymentMethod) {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYMENT_METHOD_UNAVAILABLE", "payment method is not available")
	}

	booking := &entity.Booking{
		CustomerID:         customerID,
		ServiceID:          service.ID,
		ProviderID:         service.ProviderID,
		ScheduleDate:       &scheduleDate,
		ScheduleTime:       &req.ScheduleTime,
		Address:            &req.Address,
		Notes:              req.Notes,
		SubTotal:           subTotal,
		PlatformFee:        uc.platformFee,
		Total:              total,
		PaymentMethod:      req.PaymentMethod,
		Status:             entity.BookingStatusPendingPayment,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		BookingAddOns:      bookingAddOns,
	}

	if err := uc.bookingRepo.Create(ctx, booking); err != nil {
		// A concurrent request with the same key won the unique index. The
		// winner's row is committed before the duplicate error reaches us, so
		// the lookup finds it and this caller gets the same booking, not an
		// error.
		if idempotencyKey != "" && errors.Is(err, apperror.ErrDuplicateKey) {
			existing, replayErr := uc.replayBooking(ctx, customerID, idempotencyKey, fingerprint)
			if replayErr != nil {
				return nil, replayErr
			}
			if existing != nil {
				return existing, nil
			}
		}
		log.Printf("[BookingUsecase.CreateBooking] create error: %v", err)
		return nil, err
	}

	// A provider failure is not fatal here: the payment row records the
	// failure and the customer sees the real state on the booking instead
	// of a false success.
	payment, err := uc.paymentInitiator.CreateCheckout(ctx, booking, req.PaymentMethod)
	if payment != nil {
		booking.Payment = payment
	}
	if err != nil {
		log.Printf("[BookingUsecase.CreateBooking] start checkout for booking %d: %v", booking.ID, err)
	}

	uc.notifyCustomer(ctx, booking, "booking.created", "Pesanan dibuat",
		"Pesanan dibuat. Selesaikan pembayaran agar pesanan diteruskan ke Mitra.")
	return booking, nil
}

// replayBooking returns the booking that already carries the key, or nil
// when no booking does. A key that belongs to another customer or was used
// for another request is refused, and the error never reveals that booking.
func (uc *BookingUsecase) replayBooking(ctx context.Context, customerID uint, key, fingerprint string) (*entity.Booking, error) {
	existing, err := uc.bookingRepo.FindByIdempotencyKey(ctx, key)
	if errors.Is(err, apperror.ErrBookingNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Printf("[BookingUsecase.replayBooking] find error: %v", err)
		return nil, err
	}
	if existing.CustomerID != customerID || existing.RequestFingerprint != fingerprint {
		return nil, apperror.NewDomainError(apperror.ErrAlreadyExists, "IDEMPOTENCY_KEY_REUSED", "idempotency key was already used for a different request")
	}
	return existing, nil
}

// bookingCanonicalRequest is the request a booking key is bound to. Free
// text is quoted so that a "|" inside it cannot make two different requests
// produce the same canonical string.
func bookingCanonicalRequest(req dto.CreateBookingRequest) string {
	addOnIDs := make([]string, len(req.AddOnIDs))
	for i, id := range req.AddOnIDs {
		addOnIDs[i] = strconv.FormatUint(uint64(id), 10)
	}
	return idempotency.CanonicalRequest(
		strconv.FormatUint(uint64(req.ServiceID), 10),
		strconv.Quote(req.ScheduleDate),
		strconv.Quote(req.ScheduleTime),
		strconv.Quote(req.Address),
		strconv.Quote(req.Notes),
		strconv.Quote(strings.Join(addOnIDs, ",")),
		strconv.Quote(req.PaymentMethod),
	)
}

// RetryPayment sends the customer's pending checkout again. The payment
// usecase reuses the key the attempt was first sent with, so a retry never
// starts a second charge.
func (uc *BookingUsecase) RetryPayment(ctx context.Context, customerID uint, bookingID uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, bookingID)
	if err != nil {
		log.Printf("[BookingUsecase.RetryPayment] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != customerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the customer can pay for this booking")
	}

	if booking.Status != entity.BookingStatusPendingPayment {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only retry payment for bookings awaiting payment")
	}

	if uc.paymentInitiator == nil {
		return nil, apperror.NewDomainError(apperror.ErrPaymentNotConfigured, "PAYMENT_NOT_CONFIGURED", "payment is not available right now")
	}

	payment, err := uc.paymentInitiator.RetryCheckout(ctx, booking.ID, booking.PaymentMethod)
	if err != nil {
		log.Printf("[BookingUsecase.RetryPayment] retry checkout for booking %d: %v", booking.ID, err)
		return nil, err
	}
	booking.Payment = payment
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

	if booking.Status != entity.BookingStatusPending {
		// A prepaid booking waits for the payment webhook before the Mitra
		// can act on it. The provider must never accept unpaid work.
		if booking.Status == entity.BookingStatusPendingPayment {
			return nil, apperror.NewDomainError(apperror.ErrPaymentRequired, "PAYMENT_REQUIRED", "pembayaran belum diterima")
		}
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only accept pending bookings")
	}

	if booking.Payment != nil && booking.Payment.Status != entity.PaymentStatusPaid {
		return nil, apperror.NewDomainError(apperror.ErrPaymentRequired, "PAYMENT_REQUIRED", "pembayaran belum diterima")
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
	// A paid booking that gets rejected owes the customer money back.
	uc.markRefundPending(ctx, booking)
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

	if booking.Status != entity.BookingStatusPending &&
		booking.Status != entity.BookingStatusPendingPayment &&
		booking.Status != "accepted" && booking.Status != "confirmed" {
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
	case entity.BookingStatusPending, entity.BookingStatusPendingPayment:
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
	// A paid booking that gets cancelled owes the customer money back.
	uc.markRefundPending(ctx, booking)
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
