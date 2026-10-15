package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// F5: a failed instant fan-out requeues the row for the scheduled poller
// instead of stranding it as `sending`, and surfaces the error.

func TestAdminBroadcastUsecase_CreateInstant_DeliverFailure_RequeuesAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	broadcasts.On("Create", ctx, mock.AnythingOfType("*entity.Broadcast")).Return(nil)
	broadcasts.On("ListRecipientIDs", ctx, "all", uint(0), mock.Anything).Return([]uint{1, 2}, nil)
	notifications.On("CreateMany", ctx, mock.Anything).Return(errors.New("db down"))
	broadcasts.On("Requeue", ctx, mock.Anything).Return(nil)

	_, err := uc.CreateBroadcast(ctx, 7, "Promo", "Diskon.", "all", nil)

	require.Error(t, err)
	broadcasts.AssertExpectations(t)
	broadcasts.AssertCalled(t, "Requeue", ctx, mock.Anything)
}
