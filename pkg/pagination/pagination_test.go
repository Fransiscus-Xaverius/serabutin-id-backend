package pagination

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePagination_DefaultValues(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		p := ParsePagination(c)
		return c.JSON(p)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var p Pagination
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, DefaultPage, p.Page)
	assert.Equal(t, DefaultLimit, p.Limit)
}

func TestParsePagination_CustomPage_ReturnsCorrectOffset(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		p := ParsePagination(c)
		return c.JSON(p)
	})

	req := httptest.NewRequest("GET", "/test?page=3&limit=10", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var p Pagination
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, 3, p.Page)
	assert.Equal(t, 10, p.Limit)
	assert.Equal(t, 20, p.Offset())
}

func TestParsePagination_MaxLimit_CappedAt100(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		p := ParsePagination(c)
		return c.JSON(p)
	})

	req := httptest.NewRequest("GET", "/test?limit=200", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var p Pagination
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, MaxLimit, p.Limit)
}

func TestParsePagination_NegativePage_DefaultsTo1(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		p := ParsePagination(c)
		return c.JSON(p)
	})

	req := httptest.NewRequest("GET", "/test?page=-1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var p Pagination
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, DefaultPage, p.Page)
}

func TestParsePagination_ZeroLimit_DefaultsTo20(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		p := ParsePagination(c)
		return c.JSON(p)
	})

	req := httptest.NewRequest("GET", "/test?limit=0", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var p Pagination
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, DefaultLimit, p.Limit)
}

func TestPagination_Offset_Calculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		page         int
		limit        int
		expectedOffset int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{3, 10, 20},
		{1, 20, 0},
		{5, 5, 20},
		{10, 1, 9},
	}

	for _, tc := range tests {
		p := Pagination{Page: tc.page, Limit: tc.limit}
		assert.Equal(t, tc.expectedOffset, p.Offset(), "page=%d, limit=%d", tc.page, tc.limit)
	}
}
