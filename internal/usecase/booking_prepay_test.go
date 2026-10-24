package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

// Prepaid bookings: the customer pays before the Mitra sees the order, and
// the Mitra cannot accept an unpaid booking.

type prepayFakeServiceRepo struct {
	service *entity.Service
}

func (f *prepayFakeServiceRepo) Create(ctx context.Context, s *entity.Service) error { return nil }
func (f *prepayFakeServiceRepo) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	return f.service, nil
}
func (f *prepayFakeServiceRepo) Update(ctx context.Context, s *entity.Service) error { return nil }
func (f *prepayFakeServiceRepo) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (f *prepayFakeServiceRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (f *prepayFakeServiceRepo) SetStatus(ctx context.Context, id uint, status string) error {
	return nil
}
func (f *prepayFakeServiceRepo) ListForAdmin(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}

type prepayFakeInitiator struct {
	created []*entity.Booking
	payment *entity.Payment
	err     error
}

func (f *prepayFakeInitiator) CreateCheckout(ctx context.Context, booking *entity.Booking) (*entity.Payment, error) {
	f.created = append(f.created, booking)
	return f.payment, f.err
}

type prepayFakeRefundMarker struct {
	marked []uint
}

func (f *prepayFakeRefundMarker) MarkRefundPendingForBooking(ctx context.Context, bookingID uint) error {
	f.marked = append(f.marked, bookingID)
	return nil
}

func newPrepayTestUC(t *testing.T) (*BookingUsecase, *cancelFakeBookingRepo, *prepayFakeInitiator, *prepayFakeRefundMarker, *recordingNotifier) {
	t.Helper()
	bookingRepo := &cancelFakeBookingRepo{bookings: map[uint]*entity.Booking{}}
	serviceRepo := &prepayFakeServiceRepo{service: &entity.Service{
		ID:         5,
		ProviderID: 20,
		Title:      "Bersihkan Rumah",
		BasePrice:  100000,
		Status:     "active",
		AddOns:     []entity.ServiceAddOn{{ID: 1, Name: "Alat sendiri", Price: 25000}},
	}}
	initiator := &prepayFakeInitiator{payment: &entity.Payment{ID: 1, Status: entity.PaymentStatusPending, Channel: entity.PaymentChannelQRIS}}
	refundMarker := &prepayFakeRefundMarker{}
	notifier := &recordingNotifier{}

	uc := NewBookingUsecase(bookingRepo, serviceRepo, nil, 5000, 5000, 7000, &cancelFakeCancelRepo{})
	uc.SetNotifier(notifier)
	uc.SetPaymentInitiator(initiator)
	uc.SetPaymentRefundMarker(refundMarker)
	return uc, bookingRepo, initiator, refundMarker, notifier
}

func prepayRequest(method string) dto.CreateBookingRequest {
	return dto.CreateBookingRequest{
		ServiceID:     5,
		ScheduleDate:  "2026-11-02",
		ScheduleTime:  "09:30",
		Address:       "Jl. Merdeka 1",
		Notes:         "Fokus dapur",
		AddOnIDs:      []uint{1},
		PaymentMethod: method,
	}
}

func TestCreateBooking_QRIS_CreatesPendingPaymentBookingAndCheckout(t *testing.T) {
	uc, _, initiator, _, notifier := newPrepayTestUC(t)

	booking, err := uc.CreateBooking(context.Background(), 10, prepayRequest(entity.PaymentChannelQRIS))

	require.NoError(t, err)
	assert.Equal(t, entity.BookingStatusPendingPayment, booking.Status)
	assert.Equal(t, int64(130000), booking.Total, "100000 base + 25000 add-on + 5000 platform fee")
	require.Len(t, initiator.created, 1, "the provider checkout must be started")
	require.NotNil(t, booking.Payment)
	assert.Equal(t, entity.PaymentStatusPending, booking.Payment.Status)

	// The provider must not be told about the order before the money lands.
	for _, n := range notifier.notifications {
		assert.NotEqual(t, uint(20), n.userID, "the provider is notified only after payment")
	}
	foundCustomer := false
	for _, n := range notifier.notifications {
		if n.userID == 10 {
			foundCustomer = true
		}
	}
	assert.True(t, foundCustomer, "the customer is told to complete the payment")
}

func TestCreateBooking_LegacyMethod_KeepsPendingAndCreatesNoPayment(t *testing.T) {
	uc, _, initiator, _, _ := newPrepayTestUC(t)

	booking, err := uc.CreateBooking(context.Background(), 10, prepayRequest("cash_on_service"))

	require.NoError(t, err)
	assert.Equal(t, entity.BookingStatusPending, booking.Status)
	assert.Nil(t, booking.Payment)
	assert.Empty(t, initiator.created, "legacy bookings do not start a checkout")
}

func TestCreateBooking_CheckoutFailure_StillReturnsBookingWithFailedPayment(t *testing.T) {
	uc, _, initiator, _, _ := newPrepayTestUC(t)
	initiator.err = assert.AnError
	initiator.payment = &entity.Payment{ID: 1, Status: entity.PaymentStatusFailed, LastError: "gateway down"}

	booking, err := uc.CreateBooking(context.Background(), 10, prepayRequest(entity.PaymentChannelQRIS))

	require.NoError(t, err, "the booking itself exists; the payment failure is reported through the payment object")
	assert.Equal(t, entity.BookingStatusPendingPayment, booking.Status)
	require.NotNil(t, booking.Payment)
	assert.Equal(t, entity.PaymentStatusFailed, booking.Payment.Status)
}

func TestAcceptBooking_UnpaidBooking_IsRejected(t *testing.T) {
	uc, bookingRepo, _, _, _ := newPrepayTestUC(t)
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20,
		ScheduleDate: &time.Time{}, ScheduleTime: strPtr("09:30"), Address: strPtr("Jl. Merdeka 1"),
		Status:  entity.BookingStatusPendingPayment,
		Payment: &entity.Payment{Status: entity.PaymentStatusPending},
	}

	_, err := uc.AcceptBooking(context.Background(), 20, 1, &dto.AcceptBookingRequest{})

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "PAYMENT_REQUIRED", domainErr.Code)
}

func TestAcceptBooking_PaidBooking_IsAccepted(t *testing.T) {
	uc, bookingRepo, _, _, _ := newPrepayTestUC(t)
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20,
		ScheduleDate: &time.Time{}, ScheduleTime: strPtr("09:30"), Address: strPtr("Jl. Merdeka 1"),
		Status:  entity.BookingStatusPending,
		Payment: &entity.Payment{Status: entity.PaymentStatusPaid},
	}

	booking, err := uc.AcceptBooking(context.Background(), 20, 1, &dto.AcceptBookingRequest{})

	require.NoError(t, err)
	assert.Equal(t, "accepted", booking.Status)
}

func TestRejectBooking_PaidBooking_RecordsRefundObligation(t *testing.T) {
	uc, bookingRepo, _, refundMarker, _ := newPrepayTestUC(t)
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20,
		Status:  entity.BookingStatusPending,
		Payment: &entity.Payment{Status: entity.PaymentStatusPaid},
	}

	booking, err := uc.RejectBooking(context.Background(), 20, 1)

	require.NoError(t, err)
	assert.Equal(t, "rejected", booking.Status)
	assert.Equal(t, []uint{1}, refundMarker.marked)
}

func TestCancelBooking_UnpaidPendingPayment_CancelsWithoutFee(t *testing.T) {
	uc, bookingRepo, _, _, _ := newPrepayTestUC(t)
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20,
		Status:  entity.BookingStatusPendingPayment,
		Payment: &entity.Payment{Status: entity.PaymentStatusPending},
	}

	booking, cancellation, err := uc.CancelBooking(context.Background(), 10, 1, "changed_mind", "")

	require.NoError(t, err)
	assert.Equal(t, "cancelled", booking.Status)
	require.NotNil(t, cancellation)
	assert.Equal(t, int64(0), cancellation.FeeCharged, "an unpaid booking cancels for free")
}

func TestCancelBooking_PaidBooking_RecordsRefundObligation(t *testing.T) {
	uc, bookingRepo, _, refundMarker, _ := newPrepayTestUC(t)
	bookingRepo.bookings[1] = &entity.Booking{
		ID: 1, CustomerID: 10, ProviderID: 20,
		Status:  entity.BookingStatusPending,
		Payment: &entity.Payment{Status: entity.PaymentStatusPaid},
	}

	_, _, err := uc.CancelBooking(context.Background(), 10, 1, "changed_mind", "")

	require.NoError(t, err)
	assert.Equal(t, []uint{1}, refundMarker.marked)
}

func strPtr(value string) *string {
	return &value
}
