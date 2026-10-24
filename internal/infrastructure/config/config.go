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

	CancellationFeeCustomer int64
	CancellationFeeProvider int64

	AdminEmails []string

	// Ticket 8: chat backend service-to-service proxy for admin
	// conversation metadata (metadata only, never message content).
	ChatBackendBaseURL string
	ChatServiceToken   string

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

	// DOKU Checkout (non-SNAP) prepayment. DOKU_MODE is one of
	// disabled|mock|sandbox|production. Secrets are env-only.
	DOKUMode             string
	DOKUClientID         string
	DOKUSecretKey        string
	DOKUBaseURL          string
	DOKUHubAccountID     string
	DOKUNotificationPath string
	PaymentExpiryMinutes int
	AppPublicURL         string
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
	viper.SetDefault("CANCELLATION_FEE_CUSTOMER", 5000)
	viper.SetDefault("CANCELLATION_FEE_PROVIDER", 5000)

	viper.SetDefault("STORAGE_DRIVER", "local")
	viper.SetDefault("STORAGE_LOCAL_PATH", "./uploads")
	viper.SetDefault("STORAGE_LOCAL_URL", "http://localhost:3000/uploads")
	viper.SetDefault("STORAGE_S3_USE_SSL", true)

	viper.SetDefault("DOKU_MODE", "disabled")
	viper.SetDefault("DOKU_CLIENT_ID", "")
	viper.SetDefault("DOKU_SECRET_KEY", "")
	viper.SetDefault("DOKU_BASE_URL", "https://api-sandbox.doku.com")
	viper.SetDefault("DOKU_HUB_ACCOUNT_ID", "")
	viper.SetDefault("DOKU_NOTIFICATION_PATH", "/api/v1/payments/doku/webhook")
	viper.SetDefault("PAYMENT_EXPIRY_MINUTES", 60)
	viper.SetDefault("APP_PUBLIC_URL", "http://localhost:5173")

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
	cfg.CancellationFeeCustomer = viper.GetInt64("CANCELLATION_FEE_CUSTOMER")
	cfg.CancellationFeeProvider = viper.GetInt64("CANCELLATION_FEE_PROVIDER")
	for _, e := range strings.Split(viper.GetString("ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.AdminEmails = append(cfg.AdminEmails, strings.ToLower(e))
		}
	}

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

	cfg.ChatBackendBaseURL = viper.GetString("CHAT_BACKEND_BASE_URL")
	cfg.ChatServiceToken = viper.GetString("CHAT_SERVICE_TOKEN")

	cfg.DOKUMode = strings.ToLower(strings.TrimSpace(viper.GetString("DOKU_MODE")))
	if cfg.DOKUMode == "" {
		cfg.DOKUMode = "disabled"
	}
	cfg.DOKUClientID = viper.GetString("DOKU_CLIENT_ID")
	cfg.DOKUSecretKey = viper.GetString("DOKU_SECRET_KEY")
	cfg.DOKUBaseURL = viper.GetString("DOKU_BASE_URL")
	cfg.DOKUHubAccountID = viper.GetString("DOKU_HUB_ACCOUNT_ID")
	cfg.DOKUNotificationPath = viper.GetString("DOKU_NOTIFICATION_PATH")
	cfg.PaymentExpiryMinutes = viper.GetInt("PAYMENT_EXPIRY_MINUTES")
	if cfg.PaymentExpiryMinutes <= 0 {
		cfg.PaymentExpiryMinutes = 60
	}
	cfg.AppPublicURL = strings.TrimRight(viper.GetString("APP_PUBLIC_URL"), "/")

	if err := ValidateSecrets(cfg.Environment, cfg.JWTSecret, cfg.JWTRefreshSecret); err != nil {
		return nil, err
	}
	if err := ValidatePaymentConfig(cfg.Environment, cfg.DOKUMode, cfg.DOKUClientID, cfg.DOKUSecretKey); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ValidatePaymentConfig rejects unsafe combinations: production must never
// use the mock gateway, and sandbox/production must carry real credentials.
func ValidatePaymentConfig(environment, mode, clientID, secretKey string) error {
	env := strings.ToLower(strings.TrimSpace(environment))
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "disabled"
	}

	switch mode {
	case "disabled", "mock", "sandbox", "production":
	default:
		return fmt.Errorf("DOKU_MODE must be one of disabled, mock, sandbox, production")
	}

	if env == "production" && mode == "mock" {
		return fmt.Errorf("DOKU_MODE=mock is not allowed in production")
	}
	if (mode == "sandbox" || mode == "production") && (strings.TrimSpace(clientID) == "" || strings.TrimSpace(secretKey) == "") {
		return fmt.Errorf("DOKU_CLIENT_ID and DOKU_SECRET_KEY are required when DOKU_MODE=%s", mode)
	}
	return nil
}
