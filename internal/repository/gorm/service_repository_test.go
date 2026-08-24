package gormrepo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	container, err := testpostgres.Run(ctx, "postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ServiceModel{},
		&model.ServiceImageModel{},
		&model.ServiceAddOnModel{},
		&model.ReviewModel{},
	))
	return db
}

func TestServiceRepository_ListFilters(t *testing.T) {
	db := setupServiceTestDB(t)
	baseTime := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	services := []model.ServiceModel{
		{ID: 1, ProviderID: 1, Title: "Fresh Home", Description: "Deep clean", Category: "cleaning", UnitType: "job", BasePrice: 49999, DurationMinutes: 60, ServiceAreas: "Jakarta Selatan, Depok", ServiceType: "offered", Status: "active", CreatedAt: baseTime},
		{ID: 2, ProviderID: 2, Title: "Repairs", Description: "House repair", Category: "handyman", UnitType: "job", BasePrice: 50000, DurationMinutes: 60, ServiceAreas: " jakarta , Bandung", ServiceType: "offered", Status: "active", CreatedAt: baseTime},
		{ID: 3, ProviderID: 3, Title: "Cool Air", Description: "Maintenance", Category: "ac_service", UnitType: "job", BasePrice: 100001, DurationMinutes: 60, ServiceAreas: "JAKARTA SELATAN", ServiceType: "offered", Status: "active", CreatedAt: baseTime},
		{ID: 4, ProviderID: 4, Title: "Another Clean", Description: "Cleaning", Category: "cleaning", UnitType: "job", BasePrice: 100000, DurationMinutes: 60, ServiceAreas: "Tangerang Selatan", ServiceType: "offered", Status: "active", CreatedAt: baseTime},
	}
	require.NoError(t, db.Create(&services).Error)
	reviews := []model.ReviewModel{
		{ID: 1, BookingID: 1, ServiceID: 1, CustomerID: 1, ProviderID: 1, Rating: 5, CommunicationRating: 5, QualityRating: 5, ValueRating: 5},
		{ID: 2, BookingID: 2, ServiceID: 1, CustomerID: 2, ProviderID: 1, Rating: 4, CommunicationRating: 4, QualityRating: 4, ValueRating: 4},
		{ID: 3, BookingID: 3, ServiceID: 2, CustomerID: 3, ProviderID: 2, Rating: 4, CommunicationRating: 4, QualityRating: 4, ValueRating: 4},
		{ID: 4, BookingID: 4, ServiceID: 4, CustomerID: 4, ProviderID: 4, Rating: 5, CommunicationRating: 5, QualityRating: 5, ValueRating: 5},
	}
	require.NoError(t, db.Create(&reviews).Error)
	repo := NewServiceRepository(db)

	tests := []struct {
		name   string
		filter repository.ServiceFilter
		ids    []uint
		total  int64
	}{
		{"city exact case insensitive", repository.ServiceFilter{City: " jakarta selatan ", Page: 1, Limit: 20}, []uint{3, 1}, 2},
		{"city token boundary", repository.ServiceFilter{City: "Jakarta", Page: 1, Limit: 20}, []uint{2}, 1},
		{"query category", repository.ServiceFilter{Query: "AC_SERVICE", Page: 1, Limit: 20}, []uint{3}, 1},
		{"disjoint price ranges", repository.ServiceFilter{PriceRanges: []repository.PriceRange{{Min: 0, Max: int64Pointer(49999)}, {Min: 100001}}, Page: 1, Limit: 20}, []uint{3, 1}, 2},
		{"inclusive price boundaries", repository.ServiceFilter{PriceRanges: []repository.PriceRange{{Min: 50000, Max: int64Pointer(100000)}}, Page: 1, Limit: 20}, []uint{4, 2}, 2},
		{"rating before pagination", repository.ServiceFilter{MinRating: 4.5, Page: 1, Limit: 1}, []uint{4}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, total, err := repo.List(context.Background(), tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.total, total)
			ids := make([]uint, len(rows))
			for i := range rows {
				ids[i] = rows[i].ID
			}
			assert.Equal(t, tt.ids, ids)
		})
	}
}

func TestServiceRepository_ListStableOrdering(t *testing.T) {
	db := setupServiceTestDB(t)
	createdAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	for id := uint(1); id <= 3; id++ {
		require.NoError(t, db.Create(&model.ServiceModel{ID: id, ProviderID: id, Title: "same", Description: "same", Category: "other", UnitType: "job", BasePrice: 100, DurationMinutes: 1, ServiceAreas: "Depok", ServiceType: "offered", Status: "active", CreatedAt: createdAt}).Error)
	}
	repo := NewServiceRepository(db)

	page1, total, err := repo.List(context.Background(), repository.ServiceFilter{Sort: "price_asc", Page: 1, Limit: 2})
	require.NoError(t, err)
	page2, total2, err := repo.List(context.Background(), repository.ServiceFilter{Sort: "price_asc", Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, total, total2)
	assert.Equal(t, []uint{1, 2}, []uint{page1[0].ID, page1[1].ID})
	assert.Equal(t, uint(3), page2[0].ID)
}

func TestServiceRepository_BudgetFieldsRoundTrip(t *testing.T) {
	db := setupServiceTestDB(t)
	min, max, auto := int64(100000), int64(250000), int64(150000)
	require.NoError(t, db.Create(&model.ServiceModel{
		ID:              1,
		ProviderID:      1,
		Title:           "Requested repair",
		Description:     "Repair a broken water pump",
		Category:        "handyman",
		UnitType:        "per_session",
		BasePrice:       min,
		BudgetMin:       &min,
		BudgetMax:       &max,
		AutoAcceptPrice: &auto,
		DurationMinutes: 60,
		ServiceAreas:    "Depok",
		ServiceType:     "requested",
		Status:          "active",
	}).Error)

	service, err := NewServiceRepository(db).FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, min, *service.BudgetMin)
	assert.Equal(t, max, *service.BudgetMax)
	assert.Equal(t, auto, *service.AutoAcceptPrice)
}

func int64Pointer(value int64) *int64 { return &value }
