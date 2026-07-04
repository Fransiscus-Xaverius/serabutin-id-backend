package seed

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

func Seed(db *gorm.DB, bcryptCost int, platformFee int64) error {
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

		var providers []model.UserModel
		var customers []model.UserModel
		for _, u := range users {
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
			if (i+1)%100 == 0 {
				log.Info().Int("progress", i+1).Int("total", len(services)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded services...")
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
		bookings := generateBookings(customers, savedServices, platformFee)
		for i := range bookings {
			if err := tx.Create(&bookings[i]).Error; err != nil {
				return fmt.Errorf("insert booking %d: %w", i, err)
			}
			if (i+1)%100 == 0 {
				log.Info().Int("progress", i+1).Int("total", len(bookings)).Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Msg("Seeded bookings...")
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
