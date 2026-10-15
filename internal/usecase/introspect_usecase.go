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
	userRepo     repository.UserRepository
	// killGate is the optional ticket-12 kill-switch gate for the chat
	// signal. Nil means ungated (chat_disabled=false); outage fails open.
	killGate IntrospectKillGate
}

// IntrospectKillGate is the read surface the chat signal needs.
// Implemented by *KillSwitchUsecase.
type IntrospectKillGate interface {
	IsEnabled(ctx context.Context, module string) bool
}

// SetKillSwitchGate attaches the kill-switch gate (ticket 12). Kept as a
// setter so the established constructor shape stays stable.
func (uc *IntrospectUsecase) SetKillSwitchGate(gate IntrospectKillGate) {
	uc.killGate = gate
}

func NewIntrospectUsecase(
	tokenService *service.TokenService,
	tokenRepo repository.TokenRepository,
	userRepo repository.UserRepository,
) *IntrospectUsecase {
	return &IntrospectUsecase{
		tokenService: tokenService,
		tokenRepo:    tokenRepo,
		userRepo:     userRepo,
	}
}

type IntrospectResult struct {
	Active       bool
	UserID       uint
	Email        string
	ChatBlocked  bool
	ChatDisabled bool
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

	return IntrospectResult{Active: true, UserID: claims.UserID, Email: claims.Email, ChatBlocked: uc.chatBlocked(ctx, claims.UserID), ChatDisabled: uc.chatDisabled(ctx)}
}

// chatDisabled reports the ticket-12 kill-switch chat freeze (module or
// master off). A nil gate or lookup outage fails open (not disabled) and is
// logged: the chat backend treats a missing signal as not-disabled.
func (uc *IntrospectUsecase) chatDisabled(ctx context.Context) bool {
	if uc.killGate == nil {
		return false
	}
	return !uc.killGate.IsEnabled(ctx, "chat")
}

// chatBlocked reports the ticket-8 chat-only block flag. A lookup failure
// fails open (no block) and is logged: introspect answers token validity,
// and the chat backend treats a missing signal as not-blocked.
func (uc *IntrospectUsecase) chatBlocked(ctx context.Context, userID uint) bool {
	if uc.userRepo == nil {
		return false
	}
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Printf("[IntrospectUsecase.Introspect] chat_blocked lookup failed for user %d: %v", userID, err)
		return false
	}
	return user.ChatBlocked
}
