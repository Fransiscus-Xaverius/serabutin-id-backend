package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

type MockBroadcastRepository struct {
	mock.Mock
}

func (m *MockBroadcastRepository) Create(ctx context.Context, b *entity.Broadcast) error {
	return m.Called(ctx, b).Error(0)
}

func (m *MockBroadcastRepository) FindByID(ctx context.Context, id uint) (*entity.Broadcast, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Broadcast), args.Error(1)
}

func (m *MockBroadcastRepository) List(ctx context.Context, page, limit int) ([]*entity.Broadcast, int64, error) {
	args := m.Called(ctx, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Broadcast), args.Get(1).(int64), args.Error(2)
}

func (m *MockBroadcastRepository) Cancel(ctx context.Context, id uint) (*entity.Broadcast, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Broadcast), args.Error(1)
}

func (m *MockBroadcastRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*entity.Broadcast, error) {
	args := m.Called(ctx, mock.Anything, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Broadcast), args.Error(1)
}

func (m *MockBroadcastRepository) MarkSent(ctx context.Context, id uint, recipientCount int64) (*entity.Broadcast, error) {
	args := m.Called(ctx, id, recipientCount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Broadcast), args.Error(1)
}

func (m *MockBroadcastRepository) Requeue(ctx context.Context, id uint) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockBroadcastRepository) CountRecipients(ctx context.Context, audience string) (int64, error) {
	args := m.Called(ctx, audience)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockBroadcastRepository) ListRecipientIDs(ctx context.Context, audience string, afterID uint, batch int) ([]uint, error) {
	args := m.Called(ctx, audience, afterID, batch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uint), args.Error(1)
}

type MockNotificationRepository struct {
	mock.Mock
}

func (m *MockNotificationRepository) Create(ctx context.Context, n *entity.Notification) error {
	return m.Called(ctx, n).Error(0)
}

func (m *MockNotificationRepository) CreateMany(ctx context.Context, notifications []*entity.Notification) error {
	return m.Called(ctx, notifications).Error(0)
}

func (m *MockNotificationRepository) ListByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Notification, int64, error) {
	args := m.Called(ctx, userID, page, limit)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Notification), args.Get(1).(int64), args.Error(2)
}

func (m *MockNotificationRepository) ListSinceID(ctx context.Context, userID uint, sinceID uint, limit int) ([]*entity.Notification, error) {
	args := m.Called(ctx, userID, sinceID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Notification), args.Error(1)
}

func (m *MockNotificationRepository) ListAll(ctx context.Context, page, limit int, notifType string) ([]*entity.Notification, int64, error) {
	args := m.Called(ctx, page, limit, notifType)
	if args.Get(0) == nil {
		return nil, 0, args.Error(3)
	}
	return args.Get(0).([]*entity.Notification), args.Get(1).(int64), args.Error(3)
}

func (m *MockNotificationRepository) MarkRead(ctx context.Context, userID uint, id uint) error {
	return m.Called(ctx, userID, id).Error(0)
}

var (
	_ repository.BroadcastRepository    = (*MockBroadcastRepository)(nil)
	_ repository.NotificationRepository = (*MockNotificationRepository)(nil)
)

func setupBroadcastUsecase(b *MockBroadcastRepository, n *MockNotificationRepository) *AdminBroadcastUsecase {
	return NewAdminBroadcastUsecase(b, n)
}

// Spec 010 Test Simulation: instant blast to `customer` fans out only to
// active customers with type=admin.broadcast; providers get nothing.
func TestAdminBroadcastUsecase_CreateInstant_FansOutToAudienceOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	broadcasts.On("Create", ctx, mock.Anything).Run(func(args mock.Arguments) {
		b := args.Get(1).(*entity.Broadcast)
		b.ID = 5
		assert.Equal(t, entity.BroadcastStatusSending, b.Status)
	}).Return(nil)
	// Audience resolution at send time: only customer IDs surface.
	notifications.On("CreateMany", ctx, mock.Anything).Run(func(args mock.Arguments) {
		rows := args.Get(1).([]*entity.Notification)
		require.Len(t, rows, 2)
		for _, n := range rows {
			assert.Equal(t, BroadcastNotificationType, n.Type)
			assert.Equal(t, "Promo", n.Title)
			assert.Contains(t, []uint{10, 11}, n.UserID)
		}
	}).Return(nil).Once()
	broadcasts.On("ListRecipientIDs", ctx, "customer", uint(0), recipientPageSize).Return([]uint{10, 11}, nil).Once()
	sentAt := time.Now()
	broadcasts.On("MarkSent", ctx, uint(5), int64(2)).Return(&entity.Broadcast{
		ID: 5, Title: "Promo", Body: "Diskon 20%.", Audience: "customer",
		Status: entity.BroadcastStatusSent, RecipientCount: 2, SentAt: &sentAt, CreatedBy: 7,
	}, nil).Once()

	got, err := uc.CreateBroadcast(ctx, 7, "Promo", "Diskon 20%.", "customer", nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, entity.BroadcastStatusSent, got.Status)
	assert.Equal(t, int64(2), got.RecipientCount)
	broadcasts.AssertExpectations(t)
	notifications.AssertExpectations(t)
	notifications.AssertNotCalled(t, "CreateMany", ctx, mock.MatchedBy(func(rows []*entity.Notification) bool {
		for _, n := range rows {
			if n.UserID == 20 {
				return true
			}
		}
		return false
	}))
}

// Scheduled rows store without delivering; cancel before due prevents any
// delivery; missing title/body is 400.
func TestAdminBroadcastUsecase_CreateScheduled_NoRowsUntilDue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	future := time.Now().Add(time.Hour)
	broadcasts.On("Create", ctx, mock.Anything).Run(func(args mock.Arguments) {
		b := args.Get(1).(*entity.Broadcast)
		b.ID = 6
		assert.Equal(t, entity.BroadcastStatusScheduled, b.Status)
	}).Return(nil)

	got, err := uc.CreateBroadcast(ctx, 7, "Maintenance", "Down 00-02.", "provider", &future)

	require.NoError(t, err)
	assert.Equal(t, entity.BroadcastStatusScheduled, got.Status)
	notifications.AssertNotCalled(t, "CreateMany", mock.Anything, mock.Anything)
	broadcasts.AssertNotCalled(t, "MarkSent", mock.Anything, mock.Anything, mock.Anything)
	broadcasts.AssertExpectations(t)
}

func TestAdminBroadcastUsecase_Create_MissingTitleOrBody_Is400(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		title string
		body  string
	}{
		{"missing title", "", "isi"},
		{"blank title", "   ", "isi"},
		{"missing body", "Judul", ""},
		{"blank body", "Judul", "  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			broadcasts := new(MockBroadcastRepository)
			notifications := new(MockNotificationRepository)
			uc := setupBroadcastUsecase(broadcasts, notifications)

			_, err := uc.CreateBroadcast(ctx, 7, tc.title, tc.body, "all", nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "required")
			broadcasts.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestAdminBroadcastUsecase_Create_InvalidAudience_Is400(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	_, err := uc.CreateBroadcast(ctx, 7, "Judul", "Isi", "mitra", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "audience")
	broadcasts.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAdminBroadcastUsecase_Create_PastScheduledAt_Is422(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	past := time.Now().Add(-time.Hour)
	_, err := uc.CreateBroadcast(ctx, 7, "Judul", "Isi", "all", &past)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "future")
	broadcasts.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAdminBroadcastUsecase_Cancel_Scheduled_Cancels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	broadcasts.On("Cancel", ctx, uint(6)).Return(&entity.Broadcast{ID: 6, Status: entity.BroadcastStatusCancelled}, nil).Once()

	got, err := uc.CancelBroadcast(ctx, 6)

	require.NoError(t, err)
	assert.Equal(t, entity.BroadcastStatusCancelled, got.Status)
	broadcasts.AssertExpectations(t)
	notifications.AssertNotCalled(t, "CreateMany", mock.Anything, mock.Anything)
}

func TestAdminBroadcastUsecase_SendDue_DeliversClaimedAndRequeuesFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	dueA := &entity.Broadcast{ID: 6, Title: "A", Body: "a", Audience: "all", Status: entity.BroadcastStatusSending}
	dueB := &entity.Broadcast{ID: 7, Title: "B", Body: "b", Audience: "provider", Status: entity.BroadcastStatusSending}
	broadcasts.On("ClaimDue", ctx, mock.Anything, dueClaimLimit).Return([]*entity.Broadcast{dueA, dueB}, nil).Once()
	broadcasts.On("ListRecipientIDs", ctx, "all", uint(0), recipientPageSize).Return([]uint{10}, nil).Once()
	notifications.On("CreateMany", ctx, mock.Anything).Return(nil).Once()
	broadcasts.On("MarkSent", ctx, uint(6), int64(1)).Return(dueA, nil).Once()
	// Second blast fails delivery: no MarkSent, row requeued, batch continues.
	broadcasts.On("ListRecipientIDs", ctx, "provider", uint(0), recipientPageSize).Return(nil, assert.AnError).Once()
	broadcasts.On("Requeue", ctx, uint(7)).Return(nil).Once()

	require.NoError(t, uc.SendDueBroadcasts(ctx))

	broadcasts.AssertExpectations(t)
	notifications.AssertExpectations(t)
	broadcasts.AssertNotCalled(t, "MarkSent", ctx, uint(7), mock.Anything)
}

func TestAdminBroadcastUsecase_SendDue_NothingDue_NoWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	broadcasts.On("ClaimDue", ctx, mock.Anything, dueClaimLimit).Return(nil, nil).Once()

	require.NoError(t, uc.SendDueBroadcasts(ctx))

	broadcasts.AssertExpectations(t)
	notifications.AssertNotCalled(t, "CreateMany", mock.Anything, mock.Anything)
}

func TestAdminBroadcastUsecase_Estimate_ValidatesAudience(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broadcasts := new(MockBroadcastRepository)
	notifications := new(MockNotificationRepository)
	uc := setupBroadcastUsecase(broadcasts, notifications)

	broadcasts.On("CountRecipients", ctx, "provider").Return(int64(28), nil).Once()
	total, err := uc.EstimateRecipients(ctx, "provider")
	require.NoError(t, err)
	assert.Equal(t, int64(28), total)

	_, err = uc.EstimateRecipients(ctx, "everyone")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audience")
	broadcasts.AssertExpectations(t)
}
