package seed

import (
	"fmt"
	"math/rand"
	"time"

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
