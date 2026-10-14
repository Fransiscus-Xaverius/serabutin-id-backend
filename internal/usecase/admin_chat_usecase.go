package usecase

import (
	"context"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/chatclient"
)

// ChatDirectory is the minimal read surface the admin chat-safety slice
// needs from the chat backend (metadata only, never message content).
// Implemented by infrastructure/chatclient.Client; faked in tests.
type ChatDirectory interface {
	ListConversations(ctx context.Context, userID uint, page, limit int) ([]chatclient.ConversationMeta, int64, error)
}

// ChatConversationMeta is the admin-facing conversation row: metadata only.
type ChatConversationMeta struct {
	ConversationID string
	UserAID        uint
	UserBID        uint
	MessageCount   int64
	LastActivityAt string
	Flags          int64
}

// AdminChatUsecase owns the ticket-8 chat-safety slice: conversation
// metadata proxied from the chat backend plus the ID-side
// users.chat_blocked flag enforced by the chat backend at token-exchange
// time. Like the other admin usecases it never writes audit rows directly —
// middleware.AdminAuditLogger persists the mandatory `reason`.
type AdminChatUsecase struct {
	userRepo  repository.UserRepository
	directory ChatDirectory
	timeout   time.Duration
}

func NewAdminChatUsecase(userRepo repository.UserRepository, directory ChatDirectory) *AdminChatUsecase {
	return &AdminChatUsecase{userRepo: userRepo, directory: directory, timeout: 5 * time.Second}
}

// ListConversations proxies one metadata page (filterable by user_id).
// Flags stay 0 in v1: reports key targets by uint while chat conversation
// IDs are UUIDs, so no join is possible without a schema follow-up.
func (uc *AdminChatUsecase) ListConversations(ctx context.Context, userID uint, page, limit int) ([]*ChatConversationMeta, int64, error) {
	if uc.directory == nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "CHAT_DIRECTORY_UNAVAILABLE", "chat directory is not configured")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	ctx, cancel := context.WithTimeout(ctx, uc.timeout)
	defer cancel()
	rows, total, err := uc.directory.ListConversations(ctx, userID, page, limit)
	if err != nil {
		log.Printf("[AdminChatUsecase.ListConversations] chat directory error: %v", err)
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "CHAT_DIRECTORY_UNAVAILABLE", "chat directory is unavailable")
	}
	out := make([]*ChatConversationMeta, len(rows))
	for i, r := range rows {
		out[i] = &ChatConversationMeta{
			ConversationID: r.ConversationID,
			UserAID:        r.UserAID,
			UserBID:        r.UserBID,
			MessageCount:   r.MessageCount,
			LastActivityAt: r.LastActivityAt,
			Flags:          0,
		}
	}
	return out, total, nil
}

// SetChatBlock toggles users.chat_blocked. Idempotent: an already-matching
// flag returns 200 without a write (the audit middleware still records the
// call with its mandatory reason).
func (uc *AdminChatUsecase) SetChatBlock(ctx context.Context, id uint, blocked bool) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminChatUsecase.SetChatBlock] find user error: %v", err)
		return nil, err
	}
	if user.ChatBlocked == blocked {
		return user, nil
	}
	if err := uc.userRepo.UpdateChatBlocked(ctx, id, blocked); err != nil {
		log.Printf("[AdminChatUsecase.SetChatBlock] update chat_blocked error: %v", err)
		return nil, err
	}
	user.ChatBlocked = blocked
	return user, nil
}
