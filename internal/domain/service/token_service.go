package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

type TokenService struct {
	accessSecret  string
	refreshSecret string
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewTokenService(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		accessSecret:  accessSecret,
		refreshSecret: refreshSecret,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (s *TokenService) GenerateTokenPair(userID uint, email string) (*entity.TokenDetails, error) {
	td := &entity.TokenDetails{
		AccessUUID:  uuid.NewString(),
		RefreshUUID: uuid.NewString(),
		AtExpires:   time.Now().Add(s.accessTTL),
		RtExpires:   time.Now().Add(s.refreshTTL),
	}

	var err error

	atClaims := jwt.MapClaims{
		"user_id":     userID,
		"email":      email,
		"access_uuid": td.AccessUUID,
		"exp":         td.AtExpires.Unix(),
		"iat":         time.Now().Unix(),
		"type":        "access",
	}

	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString([]byte(s.accessSecret))
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "TOKEN_GENERATION_FAILED", "failed to generate access token")
	}

	rtClaims := jwt.MapClaims{
		"user_id":      userID,
		"refresh_uuid": td.RefreshUUID,
		"exp":          td.RtExpires.Unix(),
		"iat":          time.Now().Unix(),
		"type":         "refresh",
	}

	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	td.RefreshToken, err = rt.SignedString([]byte(s.refreshSecret))
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "TOKEN_GENERATION_FAILED", "failed to generate refresh token")
	}

	return td, nil
}

func (s *TokenService) ValidateRefreshToken(tokenStr string) (uint, string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.refreshSecret), nil
	})
	if err != nil {
		return 0, "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_REFRESH_TOKEN", "refresh token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_REFRESH_TOKEN", "refresh token is invalid")
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		return 0, "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_REFRESH_TOKEN", "not a refresh token")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_REFRESH_TOKEN", "invalid user ID in token")
	}

	refreshUUID, _ := claims["refresh_uuid"].(string)

	return uint(userIDFloat), refreshUUID, nil
}

type AccessTokenClaims struct {
	UserID     uint
	Email      string
	AccessUUID string
}

func (s *TokenService) ValidateAccessToken(tokenStr string) (*AccessTokenClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.accessSecret), nil
	})
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "token is invalid or expired")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "invalid token claims")
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "access" {
		return nil, apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "not an access token")
	}

	accessUUID, _ := claims["access_uuid"].(string)
	if accessUUID == "" {
		return nil, apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "missing access_uuid")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "invalid user ID")
	}

	email, _ := claims["email"].(string)

	return &AccessTokenClaims{
		UserID:     uint(userIDFloat),
		Email:      email,
		AccessUUID: accessUUID,
	}, nil
}

func (s *TokenService) ExtractAccessUUID(tokenStr string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", apperror.NewDomainError(apperror.ErrTokenInvalid, "INVALID_TOKEN", "invalid token claims")
	}

	accessUUID, _ := claims["access_uuid"].(string)
	return accessUUID, nil
}
