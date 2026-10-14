package usecase

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"golang.org/x/crypto/bcrypt"
)

type LoginLimiter interface {
	IsLocked(ctx context.Context, identifier string) (bool, error)
	RecordFailure(ctx context.Context, identifier string) error
	Reset(ctx context.Context, identifier string) error
	RemainingAttempts(ctx context.Context, identifier string) (int, error)
}

type AuthUsecase struct {
	userRepo     repository.UserRepository
	tokenRepo    repository.TokenRepository
	tokenService *service.TokenService
	loginLimiter LoginLimiter
	bcryptCost   int
}

func NewAuthUsecase(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	tokenService *service.TokenService,
	loginLimiter LoginLimiter,
	bcryptCost int,
) *AuthUsecase {
	return &AuthUsecase{
		userRepo:     userRepo,
		tokenRepo:    tokenRepo,
		tokenService: tokenService,
		loginLimiter: loginLimiter,
		bcryptCost:   bcryptCost,
	}
}

func (uc *AuthUsecase) Register(ctx context.Context, email, password, name, whatsapp, role string) (*entity.TokenDetails, *entity.User, error) {
	exists, err := uc.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[AuthUsecase.Register] check email exists error: %v", err)
		return nil, nil, err
	}
	if exists {
		return nil, nil, apperror.NewDomainError(apperror.ErrDuplicateEmail, "EMAIL_EXISTS", "email already registered")
	}

	waExists, err := uc.userRepo.FindByWhatsApp(ctx, whatsapp)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		log.Printf("[AuthUsecase.Register] check whatsapp exists error: %v", err)
		return nil, nil, err
	}
	if waExists != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrDuplicateEmail, "WHATSAPP_EXISTS", "whatsapp already registered")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), uc.bcryptCost)
	if err != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInternalServer, "HASH_FAILED", "failed to hash password")
	}

	now := time.Now()
	user := &entity.User{
		Email:     email,
		Password:  string(hashedPassword),
		Name:      name,
		WhatsApp:  whatsapp,
		Role:      role,
		ConsentAt: &now,
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

func (uc *AuthUsecase) Login(ctx context.Context, identifier, password string) (*entity.TokenDetails, *entity.User, error) {
	locked, err := uc.loginLimiter.IsLocked(ctx, identifier)
	if err != nil {
		log.Printf("[AuthUsecase.Login] check lockout error: %v", err)
	}
	if locked {
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
	}

	var user *entity.User
	if strings.Contains(identifier, "@") {
		user, err = uc.userRepo.FindByEmail(ctx, identifier)
	} else {
		user, err = uc.userRepo.FindByWhatsApp(ctx, identifier)
	}

	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			_ = uc.loginLimiter.RecordFailure(ctx, identifier)
			return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
		}
		log.Printf("[AuthUsecase.Login] find user error: %v", err)
		return nil, nil, err
	}

	if user.Status == entity.UserStatusSuspended {
		return nil, nil, apperror.NewDomainError(apperror.ErrAccountSuspended, "ACCOUNT_SUSPENDED", "Akun ditangguhkan. Hubungi support.")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		_ = uc.loginLimiter.RecordFailure(ctx, identifier)
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
	}

	_ = uc.loginLimiter.Reset(ctx, identifier)

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

	if user.Status == entity.UserStatusSuspended {
		return nil, apperror.NewDomainError(apperror.ErrAccountSuspended, "ACCOUNT_SUSPENDED", "Akun ditangguhkan. Hubungi support.")
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
