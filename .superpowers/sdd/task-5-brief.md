### Task 5: Review Generator

**Files:**
- Create: `internal/seed/reviews.go`
- Create: `internal/seed/reviews_test.go`

**Interfaces:**
- Consumes: `[]model.BookingModel` (completed bookings)
- Produces: `func generateReviews(bookings []model.BookingModel) []model.ReviewModel` — returns 1000 reviews (one per booking)

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateReviews`
Expected: FAIL (generateReviews not defined)

- [ ] **Step 3: Write minimal implementation**

```go
package seed

import (
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func generateReviews(bookings []model.BookingModel) []model.ReviewModel {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	reviews := make([]model.ReviewModel, len(bookings))

	now := time.Now()

	for i, b := range bookings {
		rating := 3 + rng.Intn(3)
		if rating > 5 {
			rating = 5
		}

		reviews[i] = model.ReviewModel{
			BookingID:           b.ID,
			ServiceID:           b.ServiceID,
			CustomerID:          b.CustomerID,
			ProviderID:          b.ProviderID,
			Rating:              rating,
			CommunicationRating: 3 + rng.Intn(3),
			QualityRating:       3 + rng.Intn(3),
			ValueRating:         3 + rng.Intn(3),
			Comment:             randomItem(rng, reviewComments),
			CreatedAt:           now,
			UpdatedAt:           now,
		}
	}

	return reviews
}

var reviewComments = []string{
	"Pelayanan sangat memuaskan!",
	"Profesional dan tepat waktu.",
	"Harganya terjangkau, kualitas bagus.",
	"Recommended banget!",
	"Cepat dan rapi. Puas!",
	"Komunikasi baik, hasil sesuai harapan.",
	"Agak lambat tapi hasilnya ok.",
	"Sudah dua kali pakai, selalu满意.",
	"Terima kasih, pelayanan ramah.",
	"Lumayan, masih bisa ditingkatkan.",
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateReviews`
Expected: PASS

- [ ] **Step 5: Run all seed tests**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/seed/reviews.go internal/seed/reviews_test.go
git commit -m "feat(seed): add review generator (1000 reviews)"
```

---
