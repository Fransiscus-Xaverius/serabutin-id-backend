package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A payout release needs the hub's profile and source account. Until DOKU
// activates the hub sub-account both are empty and the release is refused.

func TestRequirePayoutRelease_RefusesWhenHubIsNotActivated(t *testing.T) {
	err := RequirePayoutRelease(&Config{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_HUB_PROFILE_ID")
	assert.Contains(t, err.Error(), "DOKU_SOURCE_ACCOUNT")
}

func TestRequirePayoutRelease_NamesOnlyTheMissingVariable(t *testing.T) {
	err := RequirePayoutRelease(&Config{DOKUHubProfileID: "profile-hub-1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_SOURCE_ACCOUNT")
	assert.NotContains(t, err.Error(), "DOKU_HUB_PROFILE_ID")
	assert.NotContains(t, err.Error(), "profile-hub-1")
}

func TestRequirePayoutRelease_AcceptsAConfiguredHub(t *testing.T) {
	err := RequirePayoutRelease(&Config{DOKUHubProfileID: "profile-hub-1", DOKUSourceAccount: "1234567890"})

	assert.NoError(t, err)
}
