package config

import (
	"fmt"
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

	return &cfg, nil
}
