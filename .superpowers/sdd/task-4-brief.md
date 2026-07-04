### Task 4: Booking Generator

**Files:**
- Create: `internal/seed/bookings.go`
- Create: `internal/seed/bookings_test.go`

**Interfaces:**
- Consumes: `[]model.UserModel` (customers), `[]model.ServiceModel` (all services), `data.go` pools
- Produces: `func generateBookings(customers []model.UserModel, services []model.ServiceModel) []model.BookingModel` — returns 1000 completed bookings with 1-2 add-ons each

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateBookings`
Expected: FAIL (generateBookings not defined)

- [ ] **Step 3: Write minimal implementation**

```go
package seed

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func generateBookings(customers []model.UserModel, services []model.ServiceModel) []model.BookingModel {
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
		platformFee := int64(5000)
		total := subTotal + platformFee

		booking := model.BookingModel{
			CustomerID:    customer.ID,
			ServiceID:     service.ID,
			ProviderID:    service.ProviderID,
			ScheduleDate:  scheduleDate,
			ScheduleTime:  scheduleTime,
			Address:       fmt.Sprintf("Jl. %s No. %d", customer.City, rng.Intn(200)+1),
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateBookings`
Expected: PASS

- [ ] **Step 5: Run all seed tests**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/seed/bookings.go internal/seed/bookings_test.go
git commit -m "feat(seed): add booking generator (1000 completed bookings)"
```
