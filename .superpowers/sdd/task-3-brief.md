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


