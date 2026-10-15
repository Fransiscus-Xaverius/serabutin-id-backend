package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
)

// Ticket 10: service create/update validates `category` against the active
// taxonomy (422 UNKNOWN_CATEGORY); deactivating a category blocks new
// services with that slug but leaves existing rows untouched.

type stubTaxonomyRepo struct {
	active map[string]bool
}

func (s *stubTaxonomyRepo) Create(_ context.Context, _ *entity.Category) error { return nil }
func (s *stubTaxonomyRepo) FindByID(_ context.Context, _ uint) (*entity.Category, error) {
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
}
func (s *stubTaxonomyRepo) FindBySlug(_ context.Context, _ string) (*entity.Category, error) {
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
}
func (s *stubTaxonomyRepo) ListForAdmin(_ context.Context) ([]*entity.Category, error) {
	return nil, nil
}
func (s *stubTaxonomyRepo) ListPublic(_ context.Context) ([]*entity.Category, error) {
	return nil, nil
}
func (s *stubTaxonomyRepo) ExistsActive(_ context.Context, slug string) (bool, error) {
	return s.active[slug], nil
}
func (s *stubTaxonomyRepo) Update(_ context.Context, _ *entity.Category) error { return nil }
func (s *stubTaxonomyRepo) Delete(_ context.Context, _ uint) error             { return nil }
func (s *stubTaxonomyRepo) DistinctServiceCategories(_ context.Context) ([]string, error) {
	return nil, nil
}
func (s *stubTaxonomyRepo) BackfillDistinct(_ context.Context, _ []string) (int64, error) {
	return 0, nil
}

type stubServiceRepo struct {
	created *entity.Service
}

func (s *stubServiceRepo) Create(_ context.Context, svc *entity.Service) error {
	svc.ID = 1
	s.created = svc
	return nil
}

func (s *stubServiceRepo) FindByID(_ context.Context, _ uint) (*entity.Service, error) {
	return nil, apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found")
}

func (s *stubServiceRepo) Update(_ context.Context, _ *entity.Service) error { return nil }

func (s *stubServiceRepo) List(_ context.Context, _ repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}

func (s *stubServiceRepo) FindByProviderID(_ context.Context, _ uint, _ repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}

func (s *stubServiceRepo) SetStatus(_ context.Context, _ uint, _ string) error { return nil }

func (s *stubServiceRepo) ListForAdmin(_ context.Context, _ repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}

func validServiceRequest(category string) dto.CreateServiceRequest {
	return dto.CreateServiceRequest{
		Title:           "Bersih Rumah Kilat",
		Category:        category,
		Description:     "Bersih menyeluruh dua kamar dan dapur.",
		UnitType:        "per_session",
		BasePrice:       75000,
		DurationMinutes: 120,
		Images:          []dto.ServiceImageRequest{{URL: "https://img/x.jpg"}},
		ServiceAreas:    "Depok",
	}
}

func TestService_CreateUnknownCategory422(t *testing.T) {
	uc := NewServiceUsecase(&stubServiceRepo{}, nil, nil)
	uc.SetCategoryRepository(&stubTaxonomyRepo{active: map[string]bool{"cleaning": true}})

	_, err := uc.CreateService(context.Background(), 4, validServiceRequest("nope"))
	require.Error(t, err)
	apiErr := apierror.FromDomainError(err)
	assert.Equal(t, 422, apiErr.HTTPCode)
	assert.Equal(t, "UNKNOWN_CATEGORY", apiErr.Code)

	svc, err := uc.CreateService(context.Background(), 4, validServiceRequest("cleaning"))
	require.NoError(t, err)
	assert.Equal(t, "cleaning", svc.Category)
}

func TestService_CreateDeactivatedCategoryRejected(t *testing.T) {
	uc := NewServiceUsecase(&stubServiceRepo{}, nil, nil)
	uc.SetCategoryRepository(&stubTaxonomyRepo{active: map[string]bool{}})

	_, err := uc.CreateService(context.Background(), 4, validServiceRequest("cleaning"))
	require.Error(t, err)
	apiErr := apierror.FromDomainError(err)
	assert.Equal(t, 422, apiErr.HTTPCode)
	assert.Equal(t, "UNKNOWN_CATEGORY", apiErr.Code)
}
