package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func TestParseServiceListFilter(t *testing.T) {
	app := fiber.New()
	var parsedErr error
	app.Get("/services", func(c fiber.Ctx) error {
		filter, err := parseServiceListFilter(c)
		parsedErr = err
		if err != nil {
			return err
		}
		return c.JSON(filter)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/services?price_ranges=0-49999,50000-100000,100001-&min_rating=4.5", nil))
	require.NoError(t, err)
	require.NoError(t, parsedErr)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body struct {
		PriceRanges []struct {
			Min int64
			Max *int64
		}
		MinRating float64
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.PriceRanges, 3)
	assert.Equal(t, int64(0), body.PriceRanges[0].Min)
	require.NotNil(t, body.PriceRanges[0].Max)
	assert.Equal(t, int64(49999), *body.PriceRanges[0].Max)
	assert.Nil(t, body.PriceRanges[2].Max)
	assert.Equal(t, 4.5, body.MinRating)
}

func TestParseServiceListFilter_Validation(t *testing.T) {
	tests := []struct {
		name string
		url  string
		code string
	}{
		{"price conflict", "/services?price_ranges=0-49999&min_price=1", "PRICE_FILTER_CONFLICT"},
		{"malformed range", "/services?price_ranges=abc", "INVALID_PRICE_RANGES"},
		{"empty range", "/services?price_ranges=", "INVALID_PRICE_RANGES"},
		{"descending range", "/services?price_ranges=100-50", "INVALID_PRICE_RANGES"},
		{"negative scalar", "/services?min_price=-1", "INVALID_PRICE_FILTER"},
		{"rating too low", "/services?min_rating=0.9", "INVALID_MIN_RATING"},
		{"rating too high", "/services?min_rating=5.1", "INVALID_MIN_RATING"},
		{"rating malformed", "/services?min_rating=great", "INVALID_MIN_RATING"},
		{"rating empty", "/services?min_rating=", "INVALID_MIN_RATING"},
		{"rating non-finite", "/services?min_rating=NaN", "INVALID_MIN_RATING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			var parsedErr error
			app.Get("/services", func(c fiber.Ctx) error {
				_, parsedErr = parseServiceListFilter(c)
				return parsedErr
			})

			_, err := app.Test(httptest.NewRequest("GET", tt.url, nil))
			require.NoError(t, err)
			var apiErr *apierror.APIError
			require.ErrorAs(t, parsedErr, &apiErr)
			assert.Equal(t, fiber.StatusBadRequest, apiErr.HTTPCode)
			assert.Equal(t, tt.code, apiErr.Code)
		})
	}
}

type stubImageURLResolver struct{}

func (stubImageURLResolver) Upload(context.Context, string, []byte, string) (string, error) {
	return "", nil
}

func (stubImageURLResolver) Delete(context.Context, string) error { return nil }

func (stubImageURLResolver) GetURL(string) string { return "" }

func (stubImageURLResolver) ResolveImageURL(value string) string {
	return "https://media.example.com/uploads/" + value
}

func TestServiceHandler_ToServiceResponseResolvesImageURLs(t *testing.T) {
	handler := &ServiceHandler{imageResolver: stubImageURLResolver{}}

	response := handler.toServiceResponse(t.Context(), &entity.Service{
		Images: []entity.ServiceImage{{URL: "legacy.jpg"}},
	}, nil, nil)

	require.Len(t, response.Images, 1)
	assert.Equal(t, "https://media.example.com/uploads/legacy.jpg", response.Images[0].URL)
}

func TestServiceHandler_RejectsNonHTTPImageURLs(t *testing.T) {
	handler := &ServiceHandler{}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 1)
		return c.Next()
	})
	app.Post("/services", handler.Create)
	app.Patch("/services/:id", handler.Update)

	createBody := `{"title":"Home Cleaning","category":"cleaning","description":"Complete home cleaning","unit_type":"per_session","base_price":10000,"duration_minutes":60,"service_areas":"Jakarta","images":[{"url":"blob:http://localhost/id"}]}`
	updateBody := `{"images":[{"url":"/uploads/photo.jpg"}]}`
	for _, tt := range []struct {
		method string
		path   string
		body   string
	}{
		{fiber.MethodPost, "/services", createBody},
		{fiber.MethodPatch, "/services/1", updateBody},
	} {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

		var body apierror.APIError
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "VALIDATION_ERROR", body.Code)
		require.NoError(t, resp.Body.Close())
	}
}
