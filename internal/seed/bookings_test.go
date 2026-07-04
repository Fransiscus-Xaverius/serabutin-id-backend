package seed

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/stretchr/testify/assert"
)

func TestGenerateBookings_ReturnsCorrectCount(t *testing.T) {
	customers := make([]model.UserModel, 800)
	for i := range customers {
		customers[i].ID = uint(i + 201)
		customers[i].City = "Jakarta"
		customers[i].Province = "DKI Jakarta"
	}
	services := make([]model.ServiceModel, 1000)
	for i := range services {
		services[i].ID = uint(i + 1)
		services[i].ProviderID = uint(i%200 + 1)
		services[i].BasePrice = 100000
	}
	bookings := generateBookings(customers, services)
	assert.Len(t, bookings, 1000)
}

func TestGenerateBookings_AllCompleted(t *testing.T) {
	customers := make([]model.UserModel, 800)
	for i := range customers {
		customers[i].ID = uint(i + 201)
	}
	services := make([]model.ServiceModel, 1000)
	for i := range services {
		services[i].ID = uint(i + 1)
		services[i].ProviderID = uint(i%200 + 1)
		services[i].BasePrice = 100000
	}
	bookings := generateBookings(customers, services)
	for _, b := range bookings {
		assert.Equal(t, "completed", b.Status)
	}
}
