package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

// F7: the force-cancel transaction keeps exact 422/404 semantics when the
// guarded write loses a race.

func TestAdminCancelBooking_GuardRace_TurnedTerminal_422(t *testing.T) {
	uc, bookingRepo, cancelRepo, _ := newAdminBookingTestUC("pending")
	// Simulate a concurrent terminal transition that lands between the
	// pre-check read and the guarded write.
	cancelRepo.txErr = apperror.NewDomainError(apperror.ErrInvalidTransition, "CANCEL_GUARD_FAILED", "booking turned terminal or is missing")
	bookingRepo.bookings[1].Status = "cancelled"

	_, _, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", domainErr.Code)
	assert.Empty(t, cancelRepo.created)
}

func TestAdminCancelBooking_GuardRace_BookingGone_404(t *testing.T) {
	uc, bookingRepo, cancelRepo, _ := newAdminBookingTestUC("pending")
	cancelRepo.txErr = apperror.NewDomainError(apperror.ErrInvalidTransition, "CANCEL_GUARD_FAILED", "booking turned terminal or is missing")
	delete(bookingRepo.bookings, uint(1))

	_, _, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "BOOKING_NOT_FOUND", domainErr.Code)
	assert.Empty(t, cancelRepo.created)
}

func TestAdminCancelBooking_Success_WritesSingleTxRow(t *testing.T) {
	uc, _, cancelRepo, _ := newAdminBookingTestUC("pending")

	booking, cancellation, err := uc.AdminCancelBooking(context.Background(), 7, 1, "sengketa")

	require.NoError(t, err)
	assert.Equal(t, "cancelled", booking.Status)
	require.Len(t, cancelRepo.created, 1)
	assert.Equal(t, cancellation.ID, cancelRepo.created[0].ID)
}
