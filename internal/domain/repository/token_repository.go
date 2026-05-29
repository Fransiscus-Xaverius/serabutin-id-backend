package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type TokenRepository interface {
	StoreRefreshToken(ctx context.Context, userID uint, tokenUUID string, expires time.Duration) error
	IsRefreshTokenValid(ctx context.Context, tokenUUID string) (uint, error)
	RevokeRefreshToken(ctx context.Context, tokenUUID string) error
	RevokeAllUserTokens(ctx context.Context, userID uint) error
	BlacklistAccessToken(ctx context.Context, tokenUUID string, expires time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, tokenUUID string) (bool, error)
	StoreTokenPair(ctx context.Context, td *entity.TokenDetails, userID uint) error
}
