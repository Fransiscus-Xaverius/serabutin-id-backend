package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/encryption"
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
	DOKUNotificationPath string
	// DOKUOverrideNotificationURL points notifications at another host (a
	// local tunnel) for one request. DOKU requires the URL path to match
	// DOKUNotificationPath.
	DOKUOverrideNotificationURL string
	// DOKUPaymentMethods are the channel slugs we offer at checkout
	// (e.g. "qris,ovo"). Each slug maps to a DOKU channel name at the
	// provider boundary, so opening a channel is a configuration change.
	DOKUPaymentMethods   []string
	PaymentExpiryMinutes int
	AppPublicURL         string

	// PayoutEncryptionKey encrypts Mitra payout bank details (AES-256-GCM).
	// It is required; the server refuses to start without it.
	PayoutEncryptionKey   []byte
	PayoutEncryptionKeyID string

	// Kirim DOKU SNAP (Domestic Payouts). These are separate from the
	// checkout credentials because the payout path also signs with an RSA
	// private key. The SNAP client id/secret fall back to the checkout pair.
	DOKUSnapBaseURL                string
	DOKUSnapClientID               string
	DOKUSnapClientSecret           string
	DOKUSnapPrivateKey             string
	DOKUPayoutChannelID            string
	DOKUPayoutSourceAccount        string
	DOKUPayoutSenderFirstName      string
	DOKUPayoutSenderLastName       string
	DOKUPayoutSenderPersonalID     string
	DOKUPayoutSenderPersonalIDType string
	DOKUPayoutSenderCountryCode    string
	DOKUPayoutCustomerNumber       string
	DOKUPayoutNotificationAgentKey string
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
	viper.SetDefault("DOKU_NOTIFICATION_PATH", "/api/v1/payments/doku/webhook")
	viper.SetDefault("DOKU_OVERRIDE_NOTIFICATION_URL", "")
	viper.SetDefault("DOKU_PAYMENT_METHODS", "qris")
	viper.SetDefault("PAYMENT_EXPIRY_MINUTES", 60)
	viper.SetDefault("APP_PUBLIC_URL", "http://localhost:5173")

	viper.SetDefault("PAYOUT_ENCRYPTION_KEY", "")
	viper.SetDefault("PAYOUT_ENCRYPTION_KEY_ID", "v1")

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
	cfg.DOKUSnapBaseURL = strings.TrimSpace(viper.GetString("DOKU_SNAP_BASE_URL"))
	if cfg.DOKUSnapBaseURL == "" {
		cfg.DOKUSnapBaseURL = cfg.DOKUBaseURL
	}
	cfg.DOKUNotificationPath = strings.TrimSpace(viper.GetString("DOKU_NOTIFICATION_PATH"))
	cfg.DOKUSnapClientID = strings.TrimSpace(viper.GetString("DOKU_SNAP_CLIENT_ID"))
	cfg.DOKUSnapClientSecret = strings.TrimSpace(viper.GetString("DOKU_SNAP_CLIENT_SECRET"))
	cfg.DOKUSnapPrivateKey = viper.GetString("DOKU_SNAP_PRIVATE_KEY")
	cfg.DOKUPayoutChannelID = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_CHANNEL_ID"))
	if cfg.DOKUPayoutChannelID == "" {
		cfg.DOKUPayoutChannelID = "11"
	}
	cfg.DOKUPayoutSourceAccount = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SOURCE_ACCOUNT"))
	cfg.DOKUPayoutSenderFirstName = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SENDER_FIRST_NAME"))
	cfg.DOKUPayoutSenderLastName = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SENDER_LAST_NAME"))
	cfg.DOKUPayoutSenderPersonalID = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SENDER_PERSONAL_ID"))
	cfg.DOKUPayoutSenderPersonalIDType = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SENDER_PERSONAL_ID_TYPE"))
	cfg.DOKUPayoutSenderCountryCode = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_SENDER_COUNTRY_CODE"))
	if cfg.DOKUPayoutSenderCountryCode == "" {
		cfg.DOKUPayoutSenderCountryCode = "ID"
	}
	cfg.DOKUPayoutCustomerNumber = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_CUSTOMER_NUMBER"))
	cfg.DOKUPayoutNotificationAgentKey = strings.TrimSpace(viper.GetString("DOKU_PAYOUT_NOTIFICATION_AGENT_KEY"))
	cfg.DOKUOverrideNotificationURL = strings.TrimRight(strings.TrimSpace(viper.GetString("DOKU_OVERRIDE_NOTIFICATION_URL")), "/")
	cfg.DOKUPaymentMethods = ParsePaymentChannels(viper.GetString("DOKU_PAYMENT_METHODS"))
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
	if err := ValidatePaymentChannels(cfg.DOKUPaymentMethods); err != nil {
		return nil, err
	}
	if err := ValidateNotificationOverrideURL(cfg.DOKUOverrideNotificationURL); err != nil {
		return nil, err
	}
	if err := ValidateNotificationPath(cfg.DOKUNotificationPath); err != nil {
		return nil, err
	}

	// An absent key is not fatal here: ops tools such as the seeder load the
	// config without touching payout data. The server enforces presence with
	// RequirePayoutEncryptionKey. A blank or whitespace-only value counts as
	// absent: it carries no key material, so it is not "malformed". Any other
	// value that is present must decode, or Load fails.
	if rawKey := viper.GetString("PAYOUT_ENCRYPTION_KEY"); strings.TrimSpace(rawKey) != "" {
		key, err := DecodePayoutEncryptionKey(rawKey)
		if err != nil {
			return nil, err
		}
		cfg.PayoutEncryptionKey = key
	}
	// PAYOUT_ENCRYPTION_KEY_ID defaults to "v1" here, at the config layer, so
	// a deployment that never sets it still records which key encrypted a row.
	cfg.PayoutEncryptionKeyID = strings.TrimSpace(viper.GetString("PAYOUT_ENCRYPTION_KEY_ID"))
	if cfg.PayoutEncryptionKeyID == "" {
		cfg.PayoutEncryptionKeyID = "v1"
	}

	return &cfg, nil
}

// RequirePayoutEncryptionKey enforces that the loaded config carries a usable
// payout encryption key. The server calls it at startup: there is no
// plaintext fallback, so a missing key stops the server instead of silently
// storing unprotected account numbers. The error never includes the value.
func RequirePayoutEncryptionKey(cfg *Config) error {
	if len(cfg.PayoutEncryptionKey) != encryption.KeySize {
		return fmt.Errorf("PAYOUT_ENCRYPTION_KEY is missing or invalid: it must be %d base64-encoded bytes (generate one with: openssl rand -base64 32)", encryption.KeySize)
	}
	return nil
}

// RequirePayoutRelease reports why a Kirim DOKU SNAP payout release cannot run
// yet. It names the missing variables, never their values. There is no WaaS
// activation step: these are credentials and identity fields on the existing
// DOKU merchant account.
func RequirePayoutRelease(cfg *Config) error {
	var missing []string
	if firstNonEmpty(cfg.DOKUSnapClientID, cfg.DOKUClientID) == "" {
		missing = append(missing, "DOKU_SNAP_CLIENT_ID or DOKU_CLIENT_ID")
	}
	if firstNonEmpty(cfg.DOKUSnapClientSecret, cfg.DOKUSecretKey) == "" {
		missing = append(missing, "DOKU_SNAP_CLIENT_SECRET or DOKU_SECRET_KEY")
	}
	if strings.TrimSpace(cfg.DOKUSnapPrivateKey) == "" {
		missing = append(missing, "DOKU_SNAP_PRIVATE_KEY")
	} else if err := doku.ValidatePayoutPrivateKey(cfg.DOKUSnapPrivateKey); err != nil {
		return fmt.Errorf("DOKU_SNAP_PRIVATE_KEY is not usable: %w", err)
	}
	if cfg.DOKUPayoutChannelID == "" {
		missing = append(missing, "DOKU_PAYOUT_CHANNEL_ID")
	}
	if cfg.DOKUPayoutSourceAccount == "" {
		missing = append(missing, "DOKU_PAYOUT_SOURCE_ACCOUNT")
	}
	if cfg.DOKUPayoutSenderFirstName == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_FIRST_NAME")
	}
	if cfg.DOKUPayoutSenderLastName == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_LAST_NAME")
	}
	if cfg.DOKUPayoutSenderPersonalID == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_PERSONAL_ID")
	}
	if cfg.DOKUPayoutSenderPersonalIDType == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_PERSONAL_ID_TYPE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("payout release is not configured: %s must be set", strings.Join(missing, ", "))
	}
	if err := validatePayoutBaseURL(cfg.Environment, firstNonEmpty(cfg.DOKUSnapBaseURL, cfg.DOKUBaseURL)); err != nil {
		return err
	}
	return nil
}

// validatePayoutBaseURL keeps RSA-signed transfer requests on TLS, or on a
// loopback/private host in development for the labeled SNAP stub. A public
// HTTP host is never accepted: its check-status answer decides whether a
// payout is paid.
func validatePayoutBaseURL(environment, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("DOKU_SNAP_BASE_URL or DOKU_BASE_URL must be set")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("DOKU_SNAP_BASE_URL is not a valid absolute URL")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" {
		return fmt.Errorf("DOKU_SNAP_BASE_URL must use https")
	}
	if strings.EqualFold(environment, "production") {
		return fmt.Errorf("DOKU_SNAP_BASE_URL must use https in production")
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsPrivate() {
		return nil
	}
	return fmt.Errorf("DOKU_SNAP_BASE_URL may use http only for a loopback or private host in development")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// DecodePayoutEncryptionKey decodes the base64 encryption key from the
// environment. It is strict: an empty value is an error. Errors never include
// the key value.
func DecodePayoutEncryptionKey(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY is required (generate one with: openssl rand -base64 32)")
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY must be base64: %w", err)
	}
	if len(decoded) != encryption.KeySize {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY must decode to %d bytes, got %d", encryption.KeySize, len(decoded))
	}
	return decoded, nil
}

// ParsePaymentChannels splits the comma separated channel list into slugs.
// Order is preserved: the first channel is the default one the customer
// sees selected.
func ParsePaymentChannels(raw string) []string {
	parts := strings.Split(raw, ",")
	channels := make([]string, 0, len(parts))
	for _, part := range parts {
		slug := strings.ToLower(strings.TrimSpace(part))
		if slug == "" {
			continue
		}
		channels = append(channels, slug)
	}
	return channels
}

// ValidatePaymentChannels rejects an empty or unknown channel list, so a
// typo cannot silently produce a checkout that offers no payment method.
func ValidatePaymentChannels(channels []string) error {
	if len(channels) == 0 {
		return fmt.Errorf("DOKU_PAYMENT_METHODS must list at least one payment channel")
	}
	seen := make(map[string]struct{}, len(channels))
	for _, slug := range channels {
		if _, ok := entity.DOKUChannelName(slug); !ok {
			return fmt.Errorf("unknown payment channel %q in DOKU_PAYMENT_METHODS", slug)
		}
		if _, dup := seen[slug]; dup {
			return fmt.Errorf("duplicate payment channel %q in DOKU_PAYMENT_METHODS", slug)
		}
		seen[slug] = struct{}{}
	}
	return nil
}

// notificationURLLimit is the hard cap DOKU enforces on notification URLs:
// the Back Office rejects longer values (references 06 and 07).
const notificationURLLimit = 64

// ValidateNotificationOverrideURL checks the optional per-request
// notification override against the documented URL rules: HTTPS only and at
// most 64 characters. An empty value means no override, which is valid.
func ValidateNotificationOverrideURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "https://") {
		return fmt.Errorf("DOKU_OVERRIDE_NOTIFICATION_URL must use https, got %q", trimmed)
	}
	if len(trimmed) > notificationURLLimit {
		return fmt.Errorf("DOKU_OVERRIDE_NOTIFICATION_URL must be at most %d characters, got %d", notificationURLLimit, len(trimmed))
	}
	return nil
}

// ValidateNotificationPath checks the path DOKU posts notifications to: it
// must be an absolute path so the signature Request-Target stays unambiguous.
func ValidateNotificationPath(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		// Empty falls back to the client default.
		return nil
	}
	if !strings.HasPrefix(trimmed, "/") || strings.ContainsAny(trimmed, " \t\n") {
		return fmt.Errorf("DOKU_NOTIFICATION_PATH must be an absolute path starting with /, got %q", trimmed)
	}
	return nil
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
