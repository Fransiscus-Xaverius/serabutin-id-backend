package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Ticket 4: admin jasa moderation — queue reads plus the approve / reject /
// takedown / restore state machine.

type MockServiceRepository struct {
	mock.Mock
}

func (m *MockServiceRepository) Create(ctx context.Context, service *entity.Service) error {
	return m.Called(ctx, service).Error(0)
}

func (m *MockServiceRepository) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Service), args.Error(1)
}

func (m *MockServiceRepository) Update(ctx context.Context, service *entity.Service) error {
	return m.Called(ctx, service).Error(0)
}

func (m *MockServiceRepository) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func (m *MockServiceRepository) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, providerID, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func (m *MockServiceRepository) SetStatus(ctx context.Context, id uint, status string) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *MockServiceRepository) ListForAdmin(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*entity.Service), args.Get(1).(int64), args.Error(2)
}

func TestAdminServiceUsecase_List_PassesFilterThrough(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockServiceRepository)
	uc := NewAdminServiceUsecase(mockRepo)

	rows := []*entity.Service{
		{ID: 11, ProviderID: 4, Title: "Bersih Rumah", Category: "cleaning", Status: "draft"},
	}
	filter := repository.ServiceFilter{Status: "draft", Category: "cleaning", Query: "bersih", Page: 1, Limit: 20}
	mockRepo.On("ListForAdmin", ctx, filter).Return(rows, int64(1), nil)

	got, total, err := uc.ListServices(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	assert.Equal(t, "Bersih Rumah", got[0].Title)
	mockRepo.AssertExpectations(t)
}

func TestAdminServiceUsecase_GetService_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockServiceRepository)
	uc := NewAdminServiceUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found"))

	got, err := uc.GetService(ctx, 99)

	require.Error(t, err)
	assert.Nil(t, got)
}

func TestAdminServiceUsecase_SetServiceStatus_Transitions(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name string
		from string
		to   string
	}{
		{"draft approves to active", "draft", "active"},
		{"draft rejects", "draft", "rejected"},
		{"pending approves to active", "pending", "active"},
		{"pending rejects", "pending", "rejected"},
		{"active takes down", "active", "taken_down"},
		{"active rejects", "active", "rejected"},
		{"rejected restores to active", "rejected", "active"},
		{"taken_down restores to active", "taken_down", "active"},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			mockRepo := new(MockServiceRepository)
			uc := NewAdminServiceUsecase(mockRepo)

			mockRepo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: tt.from}, nil)
			mockRepo.On("SetStatus", ctx, uint(11), tt.to).Return(nil)

			updated, err := uc.SetServiceStatus(ctx, 11, tt.to)

			require.NoError(t, err)
			require.NotNil(t, updated)
			assert.Equal(t, tt.to, updated.Status)
			mockRepo.AssertExpectations(t)
		})
	}

	invalid := []struct {
		name string
		from string
		to   string
	}{
		{"draft cannot take down directly", "draft", "taken_down"},
		{"rejected cannot take down", "rejected", "taken_down"},
		{"taken_down cannot reject", "taken_down", "rejected"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			mockRepo := new(MockServiceRepository)
			uc := NewAdminServiceUsecase(mockRepo)

			mockRepo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: tt.from}, nil)

			updated, err := uc.SetServiceStatus(ctx, 11, tt.to)

			require.Error(t, err)
			assert.Nil(t, updated)
			var domainErr *apperror.DomainError
			require.ErrorAs(t, err, &domainErr)
			assert.Equal(t, "INVALID_STATUS_TRANSITION", domainErr.Code)
			assert.ErrorIs(t, domainErr.Err, apperror.ErrInvalidTransition)
			mockRepo.AssertNotCalled(t, "SetStatus", ctx, uint(11), tt.to)
		})
	}
}

func TestAdminServiceUsecase_SetServiceStatus_SameStatus_IsNoopSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockServiceRepository)
	uc := NewAdminServiceUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(11)).Return(&entity.Service{ID: 11, Status: "active"}, nil)

	updated, err := uc.SetServiceStatus(ctx, 11, "active")

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "active", updated.Status)
	mockRepo.AssertNotCalled(t, "SetStatus", ctx, uint(11), "active")
}

func TestAdminServiceUsecase_SetServiceStatus_InvalidTarget_400(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockServiceRepository)
	uc := NewAdminServiceUsecase(mockRepo)

	updated, err := uc.SetServiceStatus(ctx, 11, "draft")

	require.Error(t, err)
	assert.Nil(t, updated)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "VALIDATION_ERROR", domainErr.Code)
	mockRepo.AssertNotCalled(t, "FindByID", ctx, uint(11))
}

func TestAdminServiceUsecase_SetServiceStatus_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockRepo := new(MockServiceRepository)
	uc := NewAdminServiceUsecase(mockRepo)

	mockRepo.On("FindByID", ctx, uint(99)).Return(nil,
		apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found"))

	updated, err := uc.SetServiceStatus(ctx, 99, "active")

	require.Error(t, err)
	assert.Nil(t, updated)
}
