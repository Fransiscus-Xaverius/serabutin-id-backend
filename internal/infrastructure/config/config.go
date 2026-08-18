package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppName     string
	Port        int
	Environment string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTSecret            string
	JWTRefreshSecret     string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration

	BcryptCost          int
	LoginMaxAttempts    int
	LoginLockoutMinutes int

	PlatformFee int64

	StorageDriver      string
	StorageLocalPath   string
	StorageLocalURL    string
	StorageS3Endpoint  string
	StorageS3Bucket    string
	StorageS3Region    string
	StorageS3AccessKey string
	StorageS3SecretKey string
	StorageS3UseSSL    bool
	StorageS3URLPrefix string
}

const (
	defaultJWTSecret        = "change-me-in-production"
	defaultJWTRefreshSecret = "change-me-refresh-in-production"
)

func ValidateSecrets(environment, jwtSecret, jwtRefreshSecret string) error {
	if jwtSecret == "" {
		return fmt.Errorf("JWT_SECRET must not be empty")
	}
	if jwtRefreshSecret == "" {
		return fmt.Errorf("JWT_REFRESH_SECRET must not be empty")
	}
	if strings.ToLower(environment) == "production" && jwtSecret == defaultJWTSecret {
		return fmt.Errorf("JWT_SECRET must not use default placeholder value in production")
	}
	if strings.ToLower(environment) == "production" && jwtRefreshSecret == defaultJWTRefreshSecret {
		return fmt.Errorf("JWT_REFRESH_SECRET must not use default placeholder value in production")
	}
	if jwtSecret != defaultJWTSecret && len(jwtSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	if jwtRefreshSecret != defaultJWTRefreshSecret && len(jwtRefreshSecret) < 32 {
		return fmt.Errorf("JWT_REFRESH_SECRET must be at least 32 bytes")
	}
	return nil
}

func Load() (*Config, error) {
	viper.AutomaticEnv()

	viper.SetDefault("APP_NAME", "serabutin-id-backend")
	viper.SetDefault("PORT", 3000)
	viper.SetDefault("ENVIRONMENT", "development")

	viper.SetDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/serabutin?sslmode=disable")

	viper.SetDefault("REDIS_ADDR", "localhost:6379")
	viper.SetDefault("REDIS_PASSWORD", "")
	viper.SetDefault("REDIS_DB", 0)

	viper.SetDefault("JWT_SECRET", "change-me-in-production")
	viper.SetDefault("JWT_REFRESH_SECRET", "change-me-refresh-in-production")
	viper.SetDefault("ACCESS_TOKEN_DURATION_MINUTES", 15)
	viper.SetDefault("REFRESH_TOKEN_DURATION_HOURS", 168) // 7 days

	viper.SetDefault("BCRYPT_COST", 12)

	viper.SetDefault("LOGIN_MAX_ATTEMPTS", 5)
	viper.SetDefault("LOGIN_LOCKOUT_MINUTES", 15)

	viper.SetDefault("PLATFORM_FEE", 5000)

	viper.SetDefault("STORAGE_DRIVER", "local")
	viper.SetDefault("STORAGE_LOCAL_PATH", "./uploads")
	viper.SetDefault("STORAGE_LOCAL_URL", "/uploads")
	viper.SetDefault("STORAGE_S3_USE_SSL", true)

	var cfg Config

	err := viper.Unmarshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.AppName = viper.GetString("APP_NAME")
	cfg.Port = viper.GetInt("PORT")
	cfg.Environment = viper.GetString("ENVIRONMENT")
	cfg.DatabaseURL = viper.GetString("DATABASE_URL")
	cfg.RedisAddr = viper.GetString("REDIS_ADDR")
	cfg.RedisPassword = viper.GetString("REDIS_PASSWORD")
	cfg.RedisDB = viper.GetInt("REDIS_DB")
	cfg.JWTSecret = viper.GetString("JWT_SECRET")
	cfg.JWTRefreshSecret = viper.GetString("JWT_REFRESH_SECRET")
	cfg.AccessTokenDuration = time.Duration(viper.GetInt("ACCESS_TOKEN_DURATION_MINUTES")) * time.Minute
	cfg.RefreshTokenDuration = time.Duration(viper.GetInt("REFRESH_TOKEN_DURATION_HOURS")) * time.Hour
	cfg.BcryptCost = viper.GetInt("BCRYPT_COST")
	cfg.LoginMaxAttempts = viper.GetInt("LOGIN_MAX_ATTEMPTS")
	cfg.LoginLockoutMinutes = viper.GetInt("LOGIN_LOCKOUT_MINUTES")
	cfg.PlatformFee = viper.GetInt64("PLATFORM_FEE")

	cfg.StorageDriver = viper.GetString("STORAGE_DRIVER")
	cfg.StorageLocalPath = viper.GetString("STORAGE_LOCAL_PATH")
	cfg.StorageLocalURL = viper.GetString("STORAGE_LOCAL_URL")
	cfg.StorageS3Endpoint = viper.GetString("STORAGE_S3_ENDPOINT")
	cfg.StorageS3Bucket = viper.GetString("STORAGE_S3_BUCKET")
	cfg.StorageS3Region = viper.GetString("STORAGE_S3_REGION")
	cfg.StorageS3AccessKey = viper.GetString("STORAGE_S3_ACCESS_KEY")
	cfg.StorageS3SecretKey = viper.GetString("STORAGE_S3_SECRET_KEY")
	cfg.StorageS3UseSSL = viper.GetBool("STORAGE_S3_USE_SSL")
	cfg.StorageS3URLPrefix = viper.GetString("STORAGE_S3_URL_PREFIX")

	if err := ValidateSecrets(cfg.Environment, cfg.JWTSecret, cfg.JWTRefreshSecret); err != nil {
		return nil, err
	}

	return &cfg, nil
}
