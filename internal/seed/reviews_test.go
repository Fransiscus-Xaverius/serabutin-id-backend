package seed

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/stretchr/testify/assert"
)

func TestGenerateReviews_ReturnsCorrectCount(t *testing.T) {
	bookings := make([]model.BookingModel, 1000)
	for i := range bookings {
		bookings[i].ID = uint(i + 1)
		bookings[i].CustomerID = uint(i + 201)
		bookings[i].ServiceID = uint(i + 1)
		bookings[i].ProviderID = uint(i%200 + 1)
	}
	reviews := generateReviews(bookings)
	assert.Len(t, reviews, 1000)
}

func TestGenerateReviews_MapsBookingFields(t *testing.T) {
	bookings := make([]model.BookingModel, 3)
	for i := range bookings {
		id := uint(i + 1)
		bookings[i] = model.BookingModel{
			ID: id, CustomerID: id + 200, ServiceID: id, ProviderID: id%200 + 1,
		}
	}
	reviews := generateReviews(bookings)
	for i, r := range reviews {
		assert.Equal(t, bookings[i].ID, r.BookingID)
		assert.Equal(t, bookings[i].ServiceID, r.ServiceID)
		assert.Equal(t, bookings[i].CustomerID, r.CustomerID)
		assert.Equal(t, bookings[i].ProviderID, r.ProviderID)
	}
}
