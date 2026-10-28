package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The payment gateway configuration must fail closed: production may never
// run the mock gateway, and the real gateway needs credentials.

func TestValidatePaymentConfig_RejectsMockInProduction(t *testing.T) {
	err := ValidatePaymentConfig("production", "mock", "client", "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock")
}

func TestValidatePaymentConfig_RequiresCredentialsForLiveModes(t *testing.T) {
	err := ValidatePaymentConfig("development", "sandbox", "", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_CLIENT_ID")
}

func TestValidatePaymentConfig_AcceptsSupportedModes(t *testing.T) {
	cases := []struct {
		name      string
		env       string
		mode      string
		clientID  string
		secretKey string
	}{
		{"disabled in development", "development", "disabled", "", ""},
		{"mock in development", "development", "mock", "", ""},
		{"sandbox with credentials", "development", "sandbox", "client", "secret"},
		{"production with credentials", "production", "production", "client", "secret"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePaymentConfig(tc.env, tc.mode, tc.clientID, tc.secretKey)
			assert.NoError(t, err)
		})
	}
}

func TestValidatePaymentConfig_RejectsUnknownMode(t *testing.T) {
	err := ValidatePaymentConfig("development", "live", "client", "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_MODE")
}

func TestParsePaymentChannels_NormalisesTheList(t *testing.T) {
	channels := ParsePaymentChannels(" qris, OVO ,dana,")

	assert.Equal(t, []string{"qris", "ovo", "dana"}, channels)
}

func TestValidatePaymentChannels_RejectsEmptyUnknownAndDuplicateChannels(t *testing.T) {
	err := ValidatePaymentChannels(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")

	err = ValidatePaymentChannels([]string{"carrier_pigeon"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "carrier_pigeon")

	err = ValidatePaymentChannels([]string{"qris", "qris"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidatePaymentChannels_AcceptsKnownChannels(t *testing.T) {
	err := ValidatePaymentChannels([]string{"qris", "ovo", "dana", "va_bca"})

	assert.NoError(t, err)
}

func TestDecodePayoutEncryptionKey_RejectsMissingMalformedAndWrongLength(t *testing.T) {
	_, err := DecodePayoutEncryptionKey("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PAYOUT_ENCRYPTION_KEY")

	_, err = DecodePayoutEncryptionKey("not base64!!")
	require.Error(t, err)

	_, err = DecodePayoutEncryptionKey(base64.StdEncoding.EncodeToString(make([]byte, 16)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecodePayoutEncryptionKey_AcceptsThirtyTwoBytes(t *testing.T) {
	key := make([]byte, 32)
	key[0] = 9

	decoded, err := DecodePayoutEncryptionKey(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	assert.Len(t, decoded, 32)
	assert.Equal(t, byte(9), decoded[0])
}

func TestLoad_AllowsAbsentPayoutKeyButRequireRejectsIt(t *testing.T) {
	t.Setenv("PAYOUT_ENCRYPTION_KEY", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.PayoutEncryptionKey)

	err = RequirePayoutEncryptionKey(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PAYOUT_ENCRYPTION_KEY")
	assert.Contains(t, err.Error(), "openssl rand -base64 32")
}

func TestLoad_RejectsMalformedPayoutKey(t *testing.T) {
	t.Setenv("PAYOUT_ENCRYPTION_KEY", "not base64!!")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PAYOUT_ENCRYPTION_KEY")

	t.Setenv("PAYOUT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 16)))

	_, err = Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestRequirePayoutEncryptionKey_AcceptsValidKeyAndNeverEchoesIt(t *testing.T) {
	require.Error(t, RequirePayoutEncryptionKey(&Config{}))

	key := make([]byte, 32)
	key[0] = 7
	require.NoError(t, RequirePayoutEncryptionKey(&Config{PayoutEncryptionKey: key}))

	short := &Config{PayoutEncryptionKey: make([]byte, 16)}
	err := RequirePayoutEncryptionKey(short)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), base64.StdEncoding.EncodeToString(short.PayoutEncryptionKey))
}
