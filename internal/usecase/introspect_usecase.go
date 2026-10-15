package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
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

	// F2: a suspended account introspects as inactive so downstream consumers
	// (chat token exchange) refuse fresh mints for it. The directory lookup
	// is the same single read that feeds the chat_blocked signal below.
	user, err := uc.lookupUser(ctx, claims.UserID)
	if err != nil {
		log.Printf("[IntrospectUsecase.Introspect] user lookup failed for user %d: %v", claims.UserID, err)
		return IntrospectResult{Active: true, UserID: claims.UserID, Email: claims.Email, ChatBlocked: false, ChatDisabled: uc.chatDisabled(ctx)}
	}
	if user != nil && user.Status == entity.UserStatusSuspended {
		log.Printf("[IntrospectUsecase.Introspect] user %d is suspended, reporting inactive", claims.UserID)
		return IntrospectResult{Active: false}
	}

	blocked := false
	if user != nil {
		blocked = user.ChatBlocked
	}
	return IntrospectResult{Active: true, UserID: claims.UserID, Email: claims.Email, ChatBlocked: blocked, ChatDisabled: uc.chatDisabled(ctx)}
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

// lookupUser reads the directory row backing both the suspended gate and
// the ticket-8 chat-only block flag. A nil userRepo (or lookup failure)
// fails open (Active without signals) and is logged: introspect answers
// token validity, and the chat backend treats a missing signal as
// not-blocked (availability-first tradeoff, see F9 doc note in openapi).
func (uc *IntrospectUsecase) lookupUser(ctx context.Context, userID uint) (*entity.User, error) {
	if uc.userRepo == nil {
		return nil, nil
	}
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}
