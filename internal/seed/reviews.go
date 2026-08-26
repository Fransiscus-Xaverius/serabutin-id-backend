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
		rating := 1 + rng.Intn(5)

		reviews[i] = model.ReviewModel{
			BookingID:           b.ID,
			ServiceID:           b.ServiceID,
			CustomerID:          b.CustomerID,
			ProviderID:          b.ProviderID,
			Rating:              rating,
			CommunicationRating: 1 + rng.Intn(5),
			QualityRating:       1 + rng.Intn(5),
			ValueRating:         1 + rng.Intn(5),
			Comment:             randomItem(rng, reviewComments),
			CreatedAt:           now.Add(time.Duration(i) * time.Minute),
			UpdatedAt:           now.Add(time.Duration(i) * time.Minute),
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
	"Sudah dua kali pakai, selalu puas.",
	"Terima kasih, pelayanan ramah.",
	"Lumayan, masih bisa ditingkatkan.",
}
