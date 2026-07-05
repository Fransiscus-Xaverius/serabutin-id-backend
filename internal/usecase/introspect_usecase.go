package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
)

type IntrospectUsecase struct {
	tokenService *service.TokenService
	tokenRepo    repository.TokenRepository
}

func NewIntrospectUsecase(
	tokenService *service.TokenService,
	tokenRepo repository.TokenRepository,
) *IntrospectUsecase {
	return &IntrospectUsecase{
		tokenService: tokenService,
		tokenRepo:    tokenRepo,
	}
}

type IntrospectResult struct {
	Active bool
	UserID uint
	Email  string
}

func (uc *IntrospectUsecase) Introspect(ctx context.Context, tokenStr string) IntrospectResult {
	claims, err := uc.tokenService.ValidateAccessToken(tokenStr)
	if err != nil {
		log.Printf("[IntrospectUsecase.Introspect] token validation failed: %v", err)
		return IntrospectResult{Active: false}
	}

	blacklisted, err := uc.tokenRepo.IsAccessTokenBlacklisted(ctx, claims.AccessUUID)
	if err != nil {
		log.Printf("[IntrospectUsecase.Introspect] blacklist check failed: %v", err)
		return IntrospectResult{Active: false}
	}
	if blacklisted {
		log.Printf("[IntrospectUsecase.Introspect] token is blacklisted for user %d", claims.UserID)
		return IntrospectResult{Active: false}
	}

	return IntrospectResult{Active: true, UserID: claims.UserID, Email: claims.Email}
}
