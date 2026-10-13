package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type bidMockBidRepo struct {
	bids []*entity.Bid
}

func (m *bidMockBidRepo) Create(ctx context.Context, bid *entity.Bid) error { return nil }
func (m *bidMockBidRepo) FindByID(ctx context.Context, id uint) (*entity.Bid, error) {
	for _, b := range m.bids {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "BID_NOT_FOUND", "bid not found")
}
func (m *bidMockBidRepo) FindByServiceID(ctx context.Context, serviceID uint) ([]*entity.Bid, error) {
	var result []*entity.Bid
	for _, b := range m.bids {
		if b.ServiceID == serviceID {
			result = append(result, b)
		}
	}
	return result, nil
}
func (m *bidMockBidRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.BidFilter) ([]*entity.Bid, int64, error) {
	var result []*entity.Bid
	for _, b := range m.bids {
		if b.ProviderID == providerID {
			result = append(result, b)
		}
	}
	return result, int64(len(result)), nil
}
func (m *bidMockBidRepo) CountByStatus(ctx context.Context, providerID uint) (map[string]int64, error) {
	return map[string]int64{"pending": 1, "accepted": 0, "rejected": 0}, nil
}
func (m *bidMockBidRepo) UpdateStatus(ctx context.Context, id uint, status string) error { return nil }
func (m *bidMockBidRepo) AcceptIfPending(ctx context.Context, id uint) error { return nil }
func (m *bidMockBidRepo) RejectOthers(ctx context.Context, serviceID uint, exceptID uint) error { return nil }

type bidMockServiceRepo struct {
	services map[uint]*entity.Service
}

func (m *bidMockServiceRepo) Create(ctx context.Context, service *entity.Service) error { return nil }
func (m *bidMockServiceRepo) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	svc, ok := m.services[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "SERVICE_NOT_FOUND", "service not found")
	}
	return svc, nil
}
func (m *bidMockServiceRepo) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (m *bidMockServiceRepo) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	return nil, 0, nil
}
func (m *bidMockServiceRepo) Update(ctx context.Context, service *entity.Service) error { return nil }
func (m *bidMockServiceRepo) SetStatus(ctx context.Context, id uint, status string) error { return nil }

type bidMockUserRepo struct {
	users map[uint]*entity.User
}

func (m *bidMockUserRepo) Create(ctx context.Context, user *entity.User) error { return nil }
func (m *bidMockUserRepo) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	user, ok := m.users[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return user, nil
}
func (m *bidMockUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	return nil, nil
}
func (m *bidMockUserRepo) FindByWhatsApp(ctx context.Context, wa string) (*entity.User, error) {
	return nil, nil
}
func (m *bidMockUserRepo) Update(ctx context.Context, user *entity.User) error { return nil }
func (m *bidMockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, nil
}

func setupBidHandlerTestApp(handler *BidHandler, authUserID uint) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		if authUserID > 0 {
			requestctx.SetUserID(c, authUserID)
		}
		return c.Next()
	})
	app.Get("/api/v1/services/:id/bids", handler.GetByService)
	app.Get("/api/v1/bids/my", handler.GetMyBids)
	return app
}

func TestBidHandler_GetByService_MetadataEnrichment(t *testing.T) {
	now := time.Now()
	service := &entity.Service{
		ID:           1,
		ProviderID:   10, // Owner is user 10 (Customer)
		Title:        "Perbaikan Atap Bocor",
		Category:     "renovation",
		Description:  "Genteng geser butuh cepat",
		ServiceAreas: "Jakarta Selatan",
		ServiceType:  "requested",
		Images: []entity.ServiceImage{
			{ID: 101, URL: "roof-repair.jpg", SortOrder: 0},
		},
	}
	bid := &entity.Bid{
		ID:         50,
		ServiceID:  1,
		ProviderID: 20, // Mitra B
		Price:      150000,
		Notes:      "Bisa datang besok pagi",
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	provider := &entity.User{
		ID:   20,
		Name: "Mitra A",
	}

	bidRepo := &bidMockBidRepo{bids: []*entity.Bid{bid}}
	serviceRepo := &bidMockServiceRepo{services: map[uint]*entity.Service{1: service}}
	userRepo := &bidMockUserRepo{users: map[uint]*entity.User{20: provider}}

	bidUsecase := usecase.NewBidUsecase(bidRepo, serviceRepo, nil, userRepo, 1000)
	profileUsecase := usecase.NewProfileUsecase(userRepo)
	serviceUsecase := usecase.NewServiceUsecase(serviceRepo, userRepo, nil)
	storageRepo := stubImageURLResolver{}

	h := NewBidHandler(bidUsecase, profileUsecase, serviceUsecase, storageRepo)
	app := setupBidHandlerTestApp(h, 10) // auth as owner (10)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/services/1/bids", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result struct {
		Success bool              `json:"success"`
		Data    []dto.BidResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.True(t, result.Success)
	require.Len(t, result.Data, 1)

	b := result.Data[0]
	assert.Equal(t, uint(50), b.ID)
	assert.Equal(t, uint(1), b.ServiceID)
	assert.Equal(t, int64(150000), b.Price)
	assert.Equal(t, "pending", b.Status)
	assert.Equal(t, "Bisa datang besok pagi", b.Notes)
	require.NotNil(t, b.Provider)
	assert.Equal(t, "Mitra A", b.Provider.Name)

	// Verify enriched service metadata
	assert.Equal(t, "Perbaikan Atap Bocor", b.ServiceTitle)
	assert.Equal(t, "renovation", b.ServiceCategory)
	assert.Equal(t, "Genteng geser butuh cepat", b.ServiceDescription)
	assert.Equal(t, "Jakarta Selatan", b.ServiceLocation)
	assert.Equal(t, "requested", b.ServiceType)
	assert.Equal(t, "https://media.example.com/uploads/roof-repair.jpg", b.ServiceImage)
}

func TestBidHandler_GetByService_ForbiddenForNonOwner(t *testing.T) {
	service := &entity.Service{
		ID:         1,
		ProviderID: 10, // Owner is 10
		Title:      "Perbaikan Atap Bocor",
	}
	bidRepo := &bidMockBidRepo{}
	serviceRepo := &bidMockServiceRepo{services: map[uint]*entity.Service{1: service}}
	userRepo := &bidMockUserRepo{}

	bidUsecase := usecase.NewBidUsecase(bidRepo, serviceRepo, nil, userRepo, 1000)
	profileUsecase := usecase.NewProfileUsecase(userRepo)
	serviceUsecase := usecase.NewServiceUsecase(serviceRepo, userRepo, nil)

	h := NewBidHandler(bidUsecase, profileUsecase, serviceUsecase, stubImageURLResolver{})
	app := setupBidHandlerTestApp(h, 99) // not owner!

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/services/1/bids", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestBidHandler_GetByService_NoImageReturnsEmpty(t *testing.T) {
	now := time.Now()
	service := &entity.Service{
		ID:           2,
		ProviderID:   10,
		Title:        "Cuci Sofa Tanpa Foto",
		Category:     "cleaning",
		ServiceAreas: "Jakarta Pusat",
		ServiceType:  "requested",
		Images:       []entity.ServiceImage{},
	}
	bid := &entity.Bid{
		ID:         51,
		ServiceID:  2,
		ProviderID: 20,
		Price:      100000,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	bidRepo := &bidMockBidRepo{bids: []*entity.Bid{bid}}
	serviceRepo := &bidMockServiceRepo{services: map[uint]*entity.Service{2: service}}
	userRepo := &bidMockUserRepo{users: map[uint]*entity.User{20: {ID: 20, Name: "Mitra A"}}}

	bidUsecase := usecase.NewBidUsecase(bidRepo, serviceRepo, nil, userRepo, 1000)
	profileUsecase := usecase.NewProfileUsecase(userRepo)
	serviceUsecase := usecase.NewServiceUsecase(serviceRepo, userRepo, nil)

	h := NewBidHandler(bidUsecase, profileUsecase, serviceUsecase, stubImageURLResolver{})
	app := setupBidHandlerTestApp(h, 10)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/services/2/bids", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result struct {
		Success bool              `json:"success"`
		Data    []dto.BidResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Len(t, result.Data, 1)
	assert.Equal(t, "Cuci Sofa Tanpa Foto", result.Data[0].ServiceTitle)
	assert.Empty(t, result.Data[0].ServiceImage)
}

func TestBidHandler_GetMyBids_ResolvesImageURL(t *testing.T) {
	now := time.Now()
	service := &entity.Service{
		ID:           3,
		ProviderID:   10, // customer is 10
		Title:        "Pengecatan Tembok",
		Category:     "renovation",
		ServiceAreas: "Bandung",
		ServiceType:  "requested",
		Images: []entity.ServiceImage{
			{ID: 1, URL: "wall-paint.png"},
		},
	}
	bid := &entity.Bid{
		ID:         75,
		ServiceID:  3,
		ProviderID: 20, // caller is provider 20
		Price:      200000,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	customer := &entity.User{
		ID:   10,
		Name: "Pelanggan B",
	}

	bidRepo := &bidMockBidRepo{bids: []*entity.Bid{bid}}
	serviceRepo := &bidMockServiceRepo{services: map[uint]*entity.Service{3: service}}
	userRepo := &bidMockUserRepo{users: map[uint]*entity.User{10: customer}}

	bidUsecase := usecase.NewBidUsecase(bidRepo, serviceRepo, nil, userRepo, 1000)
	profileUsecase := usecase.NewProfileUsecase(userRepo)
	serviceUsecase := usecase.NewServiceUsecase(serviceRepo, userRepo, nil)

	h := NewBidHandler(bidUsecase, profileUsecase, serviceUsecase, stubImageURLResolver{})
	app := setupBidHandlerTestApp(h, 20) // auth as provider 20

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/bids/my", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Data []dto.BidResponse `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Len(t, result.Data.Data, 1)

	b := result.Data.Data[0]
	assert.Equal(t, "Pengecatan Tembok", b.ServiceTitle)
	assert.Equal(t, "https://media.example.com/uploads/wall-paint.png", b.ServiceImage)
	require.NotNil(t, b.Customer)
	assert.Equal(t, "Pelanggan B", b.Customer.Name)
}
