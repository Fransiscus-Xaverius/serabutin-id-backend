# Database Seeder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone CLI tool that seeds the serabutin.id database with 1000 users, 1000 services, 1000 bookings, and 1000 reviews of realistic Indonesian test data.

**Architecture:** A Go CLI binary (`cmd/seed/main.go`) that reuses existing `config.Load()` and `database.NewPostgresDB()`. Seed logic lives in `internal/seed/` with one file per entity type plus a static data pool. A single GORM transaction wraps the entire seed — all-or-nothing. Uses existing GORM model structs from `internal/repository/model/`.

**Tech Stack:** Go 1.25, GORM, bcrypt, zerolog

## Global Constraints

- All seeding inside a single GORM transaction
- Users: 200 providers (role="provider") + 800 customers (role="customer")
- All users share password `testing123`, bcrypt at configured cost, unique salt per hash
- Services: ~5 per provider spread across 10 categories
- Bookings: 1000 total, all completed, spread across last 90 days
- Reviews: one per completed booking, 4 rating dimensions (1-5)
- Batch inserts: 100 records per batch
- Progress logged every 100 records per entity
- On success: print summary "Seeded X users, Y services, Z bookings, W reviews in T.Ts"
- On failure: roll back transaction and exit with error

---
### Task 1: Static Data Pool

**Files:**
- Create: `internal/seed/data.go`
- Create: `internal/seed/data_test.go`

**Interfaces:**
- Produces: Package-level slices and helper functions used by all seeder files

- [ ] **Step 1: Create `internal/seed/data.go` with static data pools**

```go
package seed

var firstNames = []string{
	"Ahmad", "Budi", "Citra", "Dewi", "Eko", "Fitri", "Gunawan", "Hendra",
	"Indah", "Joko", "Kartika", "Lukman", "Mega", "Nugroho", "Putri",
	"Ratna", "Sigit", "Tika", "Utami", "Wahyu", "Agus", "Bayu", "Cahyo",
	"Dian", "Eka", "Fajar", "Gita", "Hadi", "Intan", "Jaya", "Kurniawan",
	"Lestari", "Mulyadi", "Nina", "Oki", "Pramono", "Rini", "Sari", "Teguh",
	"Umar", "Vina", "Wulan", "Yoga", "Zainal", "Adi", "Bambang", "Candra",
	"Desi", "Edi", "Fani",
}

var lastNames = []string{
	"Pratama", "Wijaya", "Kusuma", "Saputra", "Santoso", "Hidayat", "Nugraha",
	"Wibowo", "Setiawan", "Susanti", "Utami", "Sari", "Wati", "Rahayu",
	"Anggraini", "Hartono", "Gunarto", "Purnomo", "Suryadi", "Ramadhani",
	"Maulana", "Syahrini", "Pertiwi", "Lestari", "Haryanto", "Maryati",
}

var cities = []string{
	"Jakarta", "Surabaya", "Bandung", "Medan", "Semarang", "Makassar", "Palembang",
	"Tangerang", "Bekasi", "Depok", "Bogor", "Yogyakarta", "Solo", "Malang",
	"Denpasar", "Banjarmasin", "Pontianak", "Padang", "Pekanbaru", "Manado",
	"Balikpapan", "Samarinda", "Aceh", "Mataram", "Kupang",
}

var provinces = []string{
	"DKI Jakarta", "Jawa Barat", "Jawa Tengah", "Jawa Timur", "Sumatera Utara",
	"Sulawesi Selatan", "Sumatera Selatan", "Banten", "DI Yogyakarta",
	"Kalimantan Timur", "Bali", "Lampung", "Riau", "Kalimantan Selatan",
	"Sumatera Barat", "Papua", "Nusa Tenggara Timur", "Kalimantan Barat",
}

var categories = []string{
	"House Cleaning", "AC Service", "Plumbing", "Electrical", "Painting",
	"Gardening", "Catering", "Photography", "Massage", "Tutoring",
}

func randomItem[T any](rng *rand.Rand, items []T) T {
	return items[rng.Intn(len(items))]
}

func randomInt(rng *rand.Rand, min, max int) int {
	return rng.Intn(max-min+1) + min
}
```

- [ ] **Step 2: Create `internal/seed/data_test.go`**

```go
package seed

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomItemReturnsItemFromSlice(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	items := []string{"a", "b", "c"}
	got := randomItem[string](rng, items)
	assert.Contains(t, items, got)
}

func TestRandomIntWithinRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		got := randomInt(rng, 5, 10)
		assert.GreaterOrEqual(t, got, 5)
		assert.LessOrEqual(t, got, 10)
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/seed/data.go internal/seed/data_test.go
git commit -m "feat(seed): add static data pools for Indonesian locale data"
```

---
### Task 2: User Generator

**Files:**
- Create: `internal/seed/users.go`
- Create: `internal/seed/users_test.go`

**Interfaces:**
- Consumes: `randomItem`, `randomInt`, data slices from `data.go`; `model.UserModel` from `internal/repository/model`
- Produces: `func generateUsers(rng *rand.Rand, bcryptCost int) ([]model.UserModel, error)` — returns 1000 users (200 providers + 800 customers) with unique emails, whatsapp numbers, and bcrypt-hashed passwords

- [ ] **Step 1: Write the failing test**

Create `internal/seed/users_test.go`:

```go
package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUsers_ReturnsCorrectCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	assert.Len(t, users, 1000)
}

func TestGenerateUsers_UniqueEmails(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	emails := make(map[string]bool)
	for _, u := range users {
		assert.False(t, emails[u.Email], "duplicate email: %s", u.Email)
		emails[u.Email] = true
	}
}

func TestGenerateUsers_UniqueWhatsApp(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	phones := make(map[string]bool)
	for _, u := range users {
		assert.False(t, phones[u.WhatsApp], "duplicate whatsapp: %s", u.WhatsApp)
		phones[u.WhatsApp] = true
	}
}

func TestGenerateUsers_HashedPasswords(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	for _, u := range users {
		assert.NotEqual(t, "testing123", u.Password)
		assert.True(t, len(u.Password) > 20, "password looks unhashed: %s", u.Password)
	}
}

func TestGenerateUsers_ProviderCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	count := 0
	for _, u := range users {
		if u.Role == "provider" {
			count++
		}
	}
	assert.Equal(t, 200, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateUsers`
Expected: compilation error (generateUsers not defined)

- [ ] **Step 3: Write minimal implementation**

Create `internal/seed/users.go`:

```go
package seed

import (
	"fmt"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"golang.org/x/crypto/bcrypt"
)

func generateUsers(bcryptCost int) ([]model.UserModel, error) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	users := make([]model.UserModel, 0, 1000)

	for i := 0; i < 1000; i++ {
		firstName := randomItem(rng, firstNames)
		lastName := randomItem(rng, lastNames)
		name := firstName + " " + lastName

		role := "customer"
		if i < 200 {
			role = "provider"
		}

		email := fmt.Sprintf("%s.%s.%d@email.com", firstName, lastName, i)
		whatsapp := fmt.Sprintf("+628%d%d", randomInt(rng, 100000000, 999999999), randomInt(rng, 10000000, 99999999))[:16]

		hash, err := bcrypt.GenerateFromPassword([]byte("testing123"), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password for user %d: %w", i, err)
		}

		city := randomItem(rng, cities)
		province := randomItem(rng, provinces)

		user := model.UserModel{
			Email:     email,
			Password:  string(hash),
			Name:      name,
			WhatsApp:  whatsapp,
			Role:      role,
			Bio:       fmt.Sprintf("Halo, saya %s. Siap membantu Anda!", name),
			Address:   fmt.Sprintf("Jl. %s No. %d", randomItem(rng, firstNames), randomInt(rng, 1, 200)),
			City:      city,
			Province:  province,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		users = append(users, user)
	}

	return users, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateUsers`
Expected: PASS

- [ ] **Step 5: Add imports and fix — ensure `time` and `math/rand` are imported**

Fix the import block in `users.go` if needed:

```go
import (
	"fmt"
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"golang.org/x/crypto/bcrypt"
)
```

- [ ] **Step 6: Run all tests again**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/seed/users.go internal/seed/users_test.go
git commit -m "feat(seed): add user generator (200 providers + 800 customers)"
```

---
### Task 3: Service Generator

**Files:**
- Create: `internal/seed/services.go`
- Create: `internal/seed/services_test.go`

**Interfaces:**
- Consumes: `data.go` pools; `model.ServiceModel`, `model.ServiceImageModel`, `model.ServiceAddOnModel`; `[]model.UserModel` (providers only) from Task 1
- Produces: `func generateServices(providers []model.UserModel) []model.ServiceModel` — returns 1000 services, each with 2-3 images and 1-3 add-ons

- [ ] **Step 1: Write the failing test**

```go
package seed

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/stretchr/testify/assert"
)

func TestGenerateServices_ReturnsCorrectCount(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	assert.Len(t, services, 1000)
}

func TestGenerateServices_AllLinkedToProviders(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	providerIDs := make(map[uint]bool)
	for _, p := range providers {
		providerIDs[p.ID] = true
	}
	for _, s := range services {
		assert.True(t, providerIDs[s.ProviderID], "service %d has unknown provider %d", s.ID, s.ProviderID)
	}
}

func TestGenerateServices_HasImages(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	for _, s := range services {
		assert.GreaterOrEqual(t, len(s.Images), 2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateServices`
Expected: FAIL (generateServices not defined)

- [ ] **Step 3: Write minimal implementation**

```go
package seed

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func generateServices(providers []model.UserModel) []model.ServiceModel {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	services := make([]model.ServiceModel, 0, 1000)

	for i := 0; i < 1000; i++ {
		provider := providers[i%len(providers)]
		category := categories[rng.Intn(len(categories))]

		title := fmt.Sprintf("%s %s", category, randomItem(rng, []string{"Profesional", "Express", "Standar", "Premium", "Hemat"}))
		unitType := randomItem(rng, []string{"session", "item", "hour", "sqm", "pcs"})
		basePrice := int64(50000 + rng.Intn(450000))
		duration := 30 + rng.Intn(210)

		service := model.ServiceModel{
			ProviderID:      provider.ID,
			Title:           title,
			Category:        category,
			Description:     fmt.Sprintf("Layanan %s terbaik untuk Anda. Berpengalaman dan terpercaya.", title),
			UnitType:        unitType,
			BasePrice:       basePrice,
			DurationMinutes: duration,
			ServiceAreas:    provider.City + ", " + provider.Province,
			Status:          "active",
			Images: []model.ServiceImageModel{
				{URL: fmt.Sprintf("https://picsum.photos/seed/svc%d/400/300", i*3), SortOrder: 0},
				{URL: fmt.Sprintf("https://picsum.photos/seed/svc%d/400/300", i*3+1), SortOrder: 1},
				{URL: fmt.Sprintf("https://picsum.photos/seed/svc%d/400/300", i*3+2), SortOrder: 2},
			},
			AddOns: []model.ServiceAddOnModel{
				{Name: randomItem(rng, []string{"Garansi 30 hari", "Free konsultasi", "Laporan detail"}), Price: int64(25000 + rng.Intn(75000))},
				{Name: randomItem(rng, []string{"Tambahan 1 jam", "Bahan free", "Prioritas"}), Price: int64(50000 + rng.Intn(100000))},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		services = append(services, service)
	}

	return services
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seed/ -v -count=1 -run TestGenerateServices`
Expected: PASS

- [ ] **Step 5: Run all seed tests**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/seed/services.go internal/seed/services_test.go
git commit -m "feat(seed): add service generator with images and add-ons"
```

---
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

---
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
### Task 6: Orchestrator + CLI Entry Point

**Files:**
- Create: `internal/seed/seed.go` — orchestrator
- Create: `cmd/seed/main.go` — CLI entry point

**Interfaces:**
- Produces: `func Seed(db *gorm.DB, bcryptCost int) error` — runs all generators in order, within a single transaction, with progress logging

- [ ] **Step 1: Write the orchestrator**

Create `internal/seed/seed.go`:

```go
package seed

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

func Seed(db *gorm.DB, bcryptCost int) error {
	start := time.Now()

	err := db.Transaction(func(tx *gorm.DB) error {
		// 1. Users
		log.Info().Msg("Generating users...")
		users, err := generateUsers(bcryptCost)
		if err != nil {
			return fmt.Errorf("generate users: %w", err)
		}
		if err := tx.CreateInBatches(users, 100).Error; err != nil {
			return fmt.Errorf("insert users: %w", err)
		}
		log.Info().Int("count", len(users)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded users")

		// Reload users to get assigned IDs
		var savedUsers []model.UserModel
		if err := tx.Find(&savedUsers).Error; err != nil {
			return fmt.Errorf("reload users: %w", err)
		}

		var providers []model.UserModel
		var customers []model.UserModel
		for _, u := range savedUsers {
			if u.Role == "provider" {
				providers = append(providers, u)
			} else {
				customers = append(customers, u)
			}
		}

		// 2. Services
		log.Info().Msg("Generating services...")
		services := generateServices(providers)
		for i := range services {
			if err := tx.Create(&services[i]).Error; err != nil {
				return fmt.Errorf("insert service %d: %w", i, err)
			}
		}
		log.Info().Int("count", len(services)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded services")

		// Reload services to get assigned IDs
		var savedServices []model.ServiceModel
		if err := tx.Preload("Images").Preload("AddOns").Find(&savedServices).Error; err != nil {
			return fmt.Errorf("reload services: %w", err)
		}

		// 3. Bookings
		log.Info().Msg("Generating bookings...")
		bookings := generateBookings(customers, savedServices)
		for i := range bookings {
			if err := tx.Create(&bookings[i]).Error; err != nil {
				return fmt.Errorf("insert booking %d: %w", i, err)
			}
		}
		log.Info().Int("count", len(bookings)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded bookings")

		// Reload bookings to get assigned IDs
		var savedBookings []model.BookingModel
		if err := tx.Preload("BookingAddOns").Find(&savedBookings).Error; err != nil {
			return fmt.Errorf("reload bookings: %w", err)
		}

		// 4. Reviews
		log.Info().Msg("Generating reviews...")
		reviews := generateReviews(savedBookings)
		if err := tx.CreateInBatches(reviews, 100).Error; err != nil {
			return fmt.Errorf("insert reviews: %w", err)
		}
		log.Info().Int("count", len(reviews)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded reviews")

		return nil
	})

	if err != nil {
		return err
	}

	log.Info().
		Str("elapsed", time.Since(start).Round(time.Millisecond).String()).
		Msg("Seeding complete")
	return nil
}
```

- [ ] **Step 2: Write the CLI entry point**

Create `cmd/seed/main.go`:

```go
package main

import (
	"log"
	"os"

	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/config"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/database"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/logger"
	"github.com/serabutin/serabutin-id-backend/internal/seed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger.Init(cfg.Environment)

	db, err := database.NewPostgresDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := seed.Seed(db, cfg.BcryptCost); err != nil {
		log.Fatalf("seeding failed: %v", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/seed/`
Expected: binary `seed.exe` built successfully (no errors)

- [ ] **Step 4: Run seed module tests only**

Run: `go test ./internal/seed/ -v -count=1`
Expected: PASS

- [ ] **Step 5: Run the whole project tests to check nothing broke**

Run: `go build ./...`
Expected: compiles without errors

- [ ] **Step 6: Commit**

```bash
git add internal/seed/seed.go cmd/seed/main.go
git commit -m "feat(seed): add orchestrator and CLI entry point"
```
