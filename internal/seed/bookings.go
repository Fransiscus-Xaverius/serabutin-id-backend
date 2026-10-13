package seed

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func generateBookings(customers []model.UserModel, services []model.ServiceModel, platformFee int64) []model.BookingModel {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	bookings := make([]model.BookingModel, 0, 1000)

	now := time.Now()

	for i := 0; i < 1000; i++ {
		customer := customers[rng.Intn(len(customers))]
		service := services[rng.Intn(len(services))]

		daysAgo := rng.Intn(90)
		scheduleDate := now.AddDate(0, 0, -daysAgo)
		hour := 8 + rng.Intn(10)
		scheduleTime := fmt.Sprintf("%02d:00", hour)

		addOns := []model.BookingAddOnModel{
			{Name: "Additional item", Price: int64(25000 + rng.Intn(50000))},
		}
		if rng.Intn(2) == 0 {
			addOns = append(addOns, model.BookingAddOnModel{
				Name: "Express service", Price: int64(50000 + rng.Intn(50000)),
			})
		}

		var addOnTotal int64
		for _, a := range addOns {
			addOnTotal += a.Price
		}

		subTotal := service.BasePrice + addOnTotal
		total := subTotal + platformFee
		address := fmt.Sprintf("Jl. %s No. %d", randomItem(rng, []string{"Merdeka", "Sudirman", "Thamrin", "Gatot Subroto", "Asia Afrika", "Diponegoro", "Ahmad Yani", "Pahlawan", "Kemerdekaan", "Sukarno Hatta"}), rng.Intn(200)+1)

		booking := model.BookingModel{
			CustomerID:    customer.ID,
			ServiceID:     service.ID,
			ProviderID:    service.ProviderID,
			ScheduleDate:  &scheduleDate,
			ScheduleTime:  &scheduleTime,
			Address:       &address,
			Notes:         fmt.Sprintf("Tolong datang tepat waktu. Terima kasih."),
			SubTotal:      subTotal,
			PlatformFee:   platformFee,
			Total:         total,
			PaymentMethod: randomItem(rng, []string{"transfer", "cash", "qris"}),
			Status:        "completed",
			BookingAddOns: addOns,
			CreatedAt:     scheduleDate.Add(-time.Duration(rng.Intn(72)) * time.Hour),
			UpdatedAt:     scheduleDate,
		}
		bookings = append(bookings, booking)
	}

	return bookings
}
