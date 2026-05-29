package usecase

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"golang.org/x/crypto/bcrypt"
)

type AuthUsecase struct {
	userRepo     repository.UserRepository
	tokenRepo    repository.TokenRepository
	tokenService *service.TokenService
	bcryptCost   int
}

func NewAuthUsecase(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	tokenService *service.TokenService,
	bcryptCost int,
) *AuthUsecase {
	return &AuthUsecase{
		userRepo:     userRepo,
		tokenRepo:    tokenRepo,
		tokenService: tokenService,
		bcryptCost:   bcryptCost,
	}
}

func (uc *AuthUsecase) Register(ctx context.Context, email, password, name string) (*entity.TokenDetails, *entity.User, error) {
	exists, err := uc.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[AuthUsecase.Register] check email exists error: %v", err)
		return nil, nil, err
	}
	if exists {
		return nil, nil, apperror.NewDomainError(apperror.ErrDuplicateEmail, "EMAIL_EXISTS", "email already registered")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), uc.bcryptCost)
	if err != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInternalServer, "HASH_FAILED", "failed to hash password")
	}

	user := &entity.User{
		Email:    email,
		Password: string(hashedPassword),
		Name:     name,
	}

	if err := uc.userRepo.Create(ctx, user); err != nil {
		log.Printf("[AuthUsecase.Register] create user error: %v", err)
		return nil, nil, err
	}

	td, err := uc.tokenService.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		log.Printf("[AuthUsecase.Register] generate tokens error: %v", err)
		return nil, nil, err
	}

	if err := uc.tokenRepo.StoreTokenPair(ctx, td, user.ID); err != nil {
		log.Printf("[AuthUsecase.Register] store tokens error: %v", err)
		return nil, nil, err
	}

	return td, user, nil
}

func (uc *AuthUsecase) Login(ctx context.Context, email, password string) (*entity.TokenDetails, *entity.User, error) {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email or password")
		}
		log.Printf("[AuthUsecase.Login] find user error: %v", err)
		return nil, nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email or password")
	}

	td, err := uc.tokenService.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		log.Printf("[AuthUsecase.Login] generate tokens error: %v", err)
		return nil, nil, err
	}

	if err := uc.tokenRepo.StoreTokenPair(ctx, td, user.ID); err != nil {
		log.Printf("[AuthUsecase.Login] store tokens error: %v", err)
		return nil, nil, err
	}

	return td, user, nil
}

func (uc *AuthUsecase) Logout(ctx context.Context, accessToken, refreshToken string) error {
	if accessToken != "" {
		accessUUID, err := uc.tokenService.ExtractAccessUUID(accessToken)
		if err == nil && accessUUID != "" {
			if err := uc.tokenRepo.BlacklistAccessToken(ctx, accessUUID, time.Hour*24); err != nil {
				log.Printf("[AuthUsecase.Logout] blacklist access token error: %v", err)
			}
		}
	}

	if refreshToken != "" {
		_, refreshUUID, err := uc.tokenService.ValidateRefreshToken(refreshToken)
		if err == nil && refreshUUID != "" {
			if err := uc.tokenRepo.RevokeRefreshToken(ctx, refreshUUID); err != nil {
				log.Printf("[AuthUsecase.Logout] revoke refresh token error: %v", err)
			}
		}
	}

	return nil
}

func (uc *AuthUsecase) RefreshToken(ctx context.Context, refreshTokenStr string) (*entity.TokenDetails, error) {
	userID, refreshUUID, err := uc.tokenService.ValidateRefreshToken(refreshTokenStr)
	if err != nil {
		return nil, err
	}

	storedUserID, err := uc.tokenRepo.IsRefreshTokenValid(ctx, refreshUUID)
	if err != nil {
		return nil, err
	}

	if storedUserID != userID {
		return nil, apperror.NewDomainError(apperror.ErrTokenRevoked, "TOKEN_REVOKED", "refresh token has been revoked")
	}

	if err := uc.tokenRepo.RevokeRefreshToken(ctx, refreshUUID); err != nil {
		log.Printf("[AuthUsecase.RefreshToken] revoke old refresh token error: %v", err)
	}

	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Printf("[AuthUsecase.RefreshToken] find user error: %v", err)
		return nil, err
	}

	td, err := uc.tokenService.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		log.Printf("[AuthUsecase.RefreshToken] generate tokens error: %v", err)
		return nil, err
	}

	if err := uc.tokenRepo.StoreTokenPair(ctx, td, user.ID); err != nil {
		log.Printf("[AuthUsecase.RefreshToken] store tokens error: %v", err)
		return nil, err
	}

	return td, nil
}
