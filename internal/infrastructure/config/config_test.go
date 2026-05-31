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
