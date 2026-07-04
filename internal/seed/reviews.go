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
