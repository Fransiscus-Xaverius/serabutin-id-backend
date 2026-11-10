package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Kirim DOKU SNAP release needs the merchant credentials, the payout source
// account and the sender identity. There is no WaaS hub activation.

func testPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func configuredPayout(t *testing.T) *Config {
	return &Config{
		DOKUBaseURL:                    "https://api-sandbox.doku.com",
		DOKUClientID:                   "BRN-TEST",
		DOKUSecretKey:                  "secret",
		DOKUSnapPrivateKey:             testPrivateKey(t),
		DOKUPayoutChannelID:            "11",
		DOKUPayoutSourceAccount:        "1234567890",
		DOKUPayoutSenderFirstName:      "Serabutin",
		DOKUPayoutSenderLastName:       "Operations",
		DOKUPayoutSenderPersonalID:     "3174000000000001",
		DOKUPayoutSenderPersonalIDType: "KTP",
	}
}

func TestRequirePayoutRelease_RefusesWithoutSnapCredentials(t *testing.T) {
	err := RequirePayoutRelease(&Config{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_SNAP_PRIVATE_KEY")
	assert.Contains(t, err.Error(), "DOKU_PAYOUT_SOURCE_ACCOUNT")
}

func TestRequirePayoutRelease_NamesOnlyTheMissingVariable(t *testing.T) {
	cfg := configuredPayout(t)
	cfg.DOKUPayoutSourceAccount = ""

	err := RequirePayoutRelease(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_PAYOUT_SOURCE_ACCOUNT")
	assert.NotContains(t, err.Error(), "DOKU_SNAP_PRIVATE_KEY")
	assert.NotContains(t, err.Error(), "1234567890")
}

func TestRequirePayoutRelease_AcceptsConfiguredSnap(t *testing.T) {
	assert.NoError(t, RequirePayoutRelease(configuredPayout(t)))
}

func TestRequirePayoutRelease_AcceptsSnapClientFallback(t *testing.T) {
	cfg := configuredPayout(t)
	cfg.DOKUClientID = ""
	cfg.DOKUSecretKey = ""
	cfg.DOKUSnapClientID = "SNAP-CLIENT"
	cfg.DOKUSnapClientSecret = "snap-secret"

	assert.NoError(t, RequirePayoutRelease(cfg))
}

func TestRequirePayoutRelease_RejectsPublicHTTP(t *testing.T) {
	cfg := configuredPayout(t)
	cfg.DOKUSnapBaseURL = "http://api-sandbox.doku.com"

	err := RequirePayoutRelease(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "http")
}

func TestRequirePayoutRelease_AcceptsPrivateHTTPForLocalStub(t *testing.T) {
	cfg := configuredPayout(t)
	cfg.Environment = "development"
	cfg.DOKUSnapBaseURL = "http://172.17.0.1:8099"

	assert.NoError(t, RequirePayoutRelease(cfg))
}

func TestRequirePayoutRelease_RejectsHTTPInProduction(t *testing.T) {
	cfg := configuredPayout(t)
	cfg.Environment = "production"
	cfg.DOKUSnapBaseURL = "http://127.0.0.1:8099"

	err := RequirePayoutRelease(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}
