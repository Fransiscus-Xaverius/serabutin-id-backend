package service

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

func setupTokenService(t *testing.T, accessTTL, refreshTTL time.Duration) *TokenService {
	t.Helper()
	return NewTokenService("access-secret-key", "refresh-secret-key", accessTTL, refreshTTL)
}

func TestGenerateTokenPair_ReturnsValidJWT(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(1, "test@example.com")

	require.NoError(t, err)
	require.NotEmpty(t, td.AccessToken)

	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(td.AccessToken, jwt.MapClaims{})
	require.NoError(t, err)

	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)

	assert.Equal(t, float64(1), claims["user_id"])
	assert.Equal(t, "test@example.com", claims["email"])
	assert.Equal(t, "access", claims["type"])
	assert.Contains(t, claims, "exp")
}

func TestGenerateTokenPair_ReturnsValidRefreshJWT(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(1, "test@example.com")

	require.NoError(t, err)
	require.NotEmpty(t, td.RefreshToken)

	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(td.RefreshToken, jwt.MapClaims{})
	require.NoError(t, err)

	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)

	assert.Equal(t, "refresh", claims["type"])
	assert.Contains(t, claims, "user_id")
	assert.Contains(t, claims, "refresh_uuid")
}

func TestGenerateTokenPair_ReturnsAllFields(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(1, "test@example.com")

	require.NoError(t, err)
	assert.NotEmpty(t, td.AccessToken)
	assert.NotEmpty(t, td.RefreshToken)
	assert.NotEmpty(t, td.AccessUUID)
	assert.NotEmpty(t, td.RefreshUUID)
	assert.False(t, td.AtExpires.IsZero())
	assert.False(t, td.RtExpires.IsZero())
}

func TestValidateRefreshToken_ValidToken_ReturnsUserIDAndUUID(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(42, "user@test.com")
	require.NoError(t, err)

	userID, refreshUUID, err := svc.ValidateRefreshToken(td.RefreshToken)

	require.NoError(t, err)
	assert.Equal(t, uint(42), userID)
	assert.Equal(t, td.RefreshUUID, refreshUUID)
}

func TestValidateRefreshToken_ExpiredToken_ReturnsError(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, -1*time.Second)

	td, err := svc.GenerateTokenPair(1, "test@example.com")
	require.NoError(t, err)

	_, _, err = svc.ValidateRefreshToken(td.RefreshToken)

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_REFRESH_TOKEN", domainErr.Code)
}

func TestValidateRefreshToken_AccessToken_ReturnsError(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(1, "test@example.com")
	require.NoError(t, err)

	_, _, err = svc.ValidateRefreshToken(td.AccessToken)

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_REFRESH_TOKEN", domainErr.Code)
}

func TestValidateRefreshToken_MalformedToken_ReturnsError(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	_, _, err := svc.ValidateRefreshToken("not-a-valid-token")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "INVALID_REFRESH_TOKEN", domainErr.Code)
}

func TestExtractAccessUUID_ValidToken_ReturnsUUID(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	td, err := svc.GenerateTokenPair(1, "test@example.com")
	require.NoError(t, err)

	accessUUID, err := svc.ExtractAccessUUID(td.AccessToken)

	require.NoError(t, err)
	assert.Equal(t, td.AccessUUID, accessUUID)
}

func TestExtractAccessUUID_MalformedToken_ReturnsError(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	_, err := svc.ExtractAccessUUID("not.a.valid.jwt")

	assert.Error(t, err)
}

func TestExtractAccessUUID_RandomString_ReturnsError(t *testing.T) {
	svc := setupTokenService(t, 15*time.Minute, 7*24*time.Hour)

	_, err := svc.ExtractAccessUUID(strings.Repeat("a", 50))

	assert.Error(t, err)
}
