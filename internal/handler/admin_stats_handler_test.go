package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
)

// Ticket 7: GET /admin/stats — read-only platform aggregates.

type statsFakeStore struct {
	users       repository.AdminUserStats
	bookings    repository.AdminBookingStats
	services    repository.AdminServiceStats
	openReports int64
}

func (f *statsFakeStore) UserStats(ctx context.Context, newSince time.Time) (repository.AdminUserStats, error) {
	return f.users, nil
}

func (f *statsFakeStore) BookingStats(ctx context.Context, since time.Time) (repository.AdminBookingStats, error) {
	return f.bookings, nil
}

func (f *statsFakeStore) ServiceStats(ctx context.Context) (repository.AdminServiceStats, error) {
	return f.services, nil
}

func (f *statsFakeStore) OpenReportCount(ctx context.Context) (int64, error) {
	return f.openReports, nil
}

func newAdminStatsTestApp(store repository.AdminStatsRepository) *fiber.App {
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminStatsUC(usecase.NewAdminStatsUsecase(store))
	app := fiber.New()
	app.Get("/api/v1/admin/stats", h.GetStats)
	return app
}

func TestAdminHandler_GetStats_ReturnsAggregates(t *testing.T) {
	t.Parallel()
	app := newAdminStatsTestApp(&statsFakeStore{
		users: repository.AdminUserStats{
			Total:  10,
			ByRole: map[string]int64{"customer": 7, "provider": 2, "admin": 1},
			New7d:  3,
		},
		bookings: repository.AdminBookingStats{
			ByStatus:     map[string]int64{"completed": 8, "cancelled": 2},
			Total30d:     10,
			Cancelled30d: 2,
			GMV30d:       800000,
			Fees30d:      50000,
		},
		services:    repository.AdminServiceStats{PendingCount: 1, ActiveCount: 4},
		openReports: 2,
	})

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	data := decoded["data"].(map[string]interface{})
	users := data["users"].(map[string]interface{})
	assert.Equal(t, float64(10), users["total"])
	assert.Equal(t, float64(3), users["new_7d"])
	assert.Equal(t, float64(2), users["by_role"].(map[string]interface{})["provider"])
	bookings := data["bookings"].(map[string]interface{})
	assert.Equal(t, float64(10), bookings["total_30d"])
	assert.Equal(t, 0.2, bookings["cancel_rate_30d"])
	assert.Equal(t, float64(8), bookings["by_status"].(map[string]interface{})["completed"])
	gmv := data["gmv"].(map[string]interface{})
	assert.Equal(t, float64(800000), gmv["total_30d"])
	assert.Equal(t, float64(50000), gmv["platform_fees_30d"])
	services := data["services"].(map[string]interface{})
	assert.Equal(t, float64(1), services["pending_count"])
	assert.Equal(t, float64(4), services["active_count"])
	reports := data["reports"].(map[string]interface{})
	assert.Equal(t, float64(2), reports["open_count"])
}

func TestAdminHandler_GetStats_EmptyStoreYieldsZerosNotNulls(t *testing.T) {
	t.Parallel()
	app := newAdminStatsTestApp(&statsFakeStore{})

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	data := decoded["data"].(map[string]interface{})
	users := data["users"].(map[string]interface{})
	assert.Equal(t, float64(0), users["total"])
	assert.NotNil(t, users["by_role"])
	bookings := data["bookings"].(map[string]interface{})
	assert.Equal(t, float64(0), bookings["cancel_rate_30d"])
	assert.NotNil(t, bookings["by_status"])
}
