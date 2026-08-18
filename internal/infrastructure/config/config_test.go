package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultLoginLimiterValues(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.LoginMaxAttempts)
	assert.Equal(t, 15, cfg.LoginLockoutMinutes)
}

func TestLoad_CustomLoginLimiterValues(t *testing.T) {
	os.Setenv("LOGIN_MAX_ATTEMPTS", "3")
	os.Setenv("LOGIN_LOCKOUT_MINUTES", "30")
	t.Cleanup(func() {
		os.Unsetenv("LOGIN_MAX_ATTEMPTS")
		os.Unsetenv("LOGIN_LOCKOUT_MINUTES")
	})

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.LoginMaxAttempts)
	assert.Equal(t, 30, cfg.LoginLockoutMinutes)
}

func TestValidateSecrets(t *testing.T) {
	strong := "0123456789abcdef0123456789abcdef"

	tests := []struct {
		name      string
		env       string
		access    string
		refresh   string
		wantError bool
	}{
		{"empty secrets", "development", "", "", true},
		{"short access secret", "development", "short", "0123456789abcdef0123456789abcdef", true},
		{"short refresh secret", "development", "0123456789abcdef0123456789abcdef", "short", true},
		{"defaults in dev are allowed", "development", "change-me-in-production", "change-me-refresh-in-production", false},
		{"defaults in production rejected", "production", "change-me-in-production", "change-me-refresh-in-production", true},
		{"strong secrets in production", "production", strong, strong, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecrets(tt.env, tt.access, tt.refresh)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoad_RejectsDefaultSecretsInProduction(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("JWT_REFRESH_SECRET")
	t.Cleanup(func() {
		os.Unsetenv("ENVIRONMENT")
	})

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_AcceptsStrongSecretsInProduction(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	os.Setenv("JWT_REFRESH_SECRET", "fedcba9876543210fedcba9876543210")
	t.Cleanup(func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
	})

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg.JWTSecret)
}
