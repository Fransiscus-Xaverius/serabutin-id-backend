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
