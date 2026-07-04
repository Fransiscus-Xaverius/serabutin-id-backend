package main

import (
	"log"

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

	if err := seed.Seed(db, cfg.BcryptCost, cfg.PlatformFee); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
}
