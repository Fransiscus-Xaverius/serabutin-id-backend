package storage

import (
	"fmt"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/config"
	"github.com/serabutin/serabutin-id-backend/internal/repository/local"
	"github.com/serabutin/serabutin-id-backend/internal/repository/s3"
)

func NewStorageRepository(cfg *config.Config) (repository.StorageRepository, error) {
	switch cfg.StorageDriver {
	case "local":
		return local.NewLocalStorage(cfg.StorageLocalPath, cfg.StorageLocalURL)
	case "s3", "minio":
		return s3.NewS3Storage(
			cfg.StorageS3Endpoint,
			cfg.StorageS3Bucket,
			cfg.StorageS3Region,
			cfg.StorageS3AccessKey,
			cfg.StorageS3SecretKey,
			cfg.StorageS3UseSSL,
			cfg.StorageS3URLPrefix,
		)
	default:
		return nil, fmt.Errorf("unknown storage driver: %s", cfg.StorageDriver)
	}
}
