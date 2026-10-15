package usecase

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// BroadcastNotificationType is the notifications.type written for every
// blast recipient. The existing admin Notifications panel and user
// notification list already render these rows — broadcasts reuse them as
// the v1 delivery channel (no push/email/SMS).
const BroadcastNotificationType = "admin.broadcast"

// recipientPageSize bounds one fan-out page: recipient IDs are keyset-paged
// and notifications inserted per page, so a blast to N users never holds N
// rows in memory at once.
const recipientPageSize = 500

// dueClaimLimit bounds how many scheduled broadcasts one ticker tick picks up.
const dueClaimLimit = 10

// AdminBroadcastUsecase owns broadcast compose/list/cancel plus fan-out
// delivery. Every mutation is audit-logged by
// middleware.AdminAuditLogger, so the handler enforces mandatory
// title/body/audience and this layer never writes audit rows directly.
type AdminBroadcastUsecase struct {
	broadcasts    repository.BroadcastRepository
	notifications repository.NotificationRepository
	// killGate is the optional ticket-12 kill-switch gate. Nil means
	// ungated (fail-open); when set and the broadcasts module (or master)
	// is off, compose is rejected and the due-poller skips delivery.
	killGate KillSwitchGate
}

// KillSwitchGate is the read surface the broadcast gate needs. Implemented
// by *KillSwitchUsecase. Defined here so this package never imports itself
// cyclically through middleware.
type KillSwitchGate interface {
	IsEnabled(ctx context.Context, module string) bool
}

// SetKillSwitchGate attaches the kill-switch gate (ticket 12). Kept as a
// setter so the established constructor shape stays stable.
func (uc *AdminBroadcastUsecase) SetKillSwitchGate(gate KillSwitchGate) {
	uc.killGate = gate
}

// broadcastsDisabled reports the ticket-12 broadcasts freeze. Nil gate or
// checker outage fails open (delivery allowed).
func (uc *AdminBroadcastUsecase) broadcastsDisabled(ctx context.Context) bool {
	if uc.killGate == nil {
		return false
	}
	return !uc.killGate.IsEnabled(ctx, "broadcasts")
}

func NewAdminBroadcastUsecase(broadcasts repository.BroadcastRepository, notifications repository.NotificationRepository) *AdminBroadcastUsecase {
	return &AdminBroadcastUsecase{broadcasts: broadcasts, notifications: notifications}
}

func validateBroadcastInput(title, body, audience string) (string, string, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	audience = strings.TrimSpace(audience)
	if title == "" {
		return "", "", apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title is required")
	}
	if len(title) > 255 {
		return "", "", apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title must be at most 255 characters")
	}
	if body == "" {
		return "", "", apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "body is required")
	}
	if len(body) > 1000 {
		return "", "", apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "body must be at most 1000 characters")
	}
	switch audience {
	case entity.BroadcastAudienceAll, entity.BroadcastAudienceCustomer, entity.BroadcastAudienceProvider:
	default:
		return "", "", apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "audience must be all, customer, or provider")
	}
	return title, body, nil
}

// CreateBroadcast composes a blast. Without scheduledAt it sends
// synchronously in-request (status sending -> sent); with a future
// scheduledAt it stores a scheduled row for the due-poller. A past
// scheduledAt is rejected with 422. A failed instant delivery requeues the
// row to scheduled (due immediately) for the poller instead of stranding it
// as sending; stuck sending rows older than 10m are additionally reclaimed
// by ClaimDue. Compose is not idempotent: every call creates a new blast.
func (uc *AdminBroadcastUsecase) CreateBroadcast(ctx context.Context, adminID uint, title, body, audience string, scheduledAt *time.Time) (*entity.Broadcast, error) {
	title, body, err := validateBroadcastInput(title, body, audience)
	if err != nil {
		return nil, err
	}
	audience = strings.TrimSpace(audience)
	if scheduledAt != nil && !scheduledAt.After(time.Now()) {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "scheduled_at must be in the future")
	}
	// Ticket 12: frozen broadcasts reject compose with 503 MODULE_DISABLED.
	if uc.broadcastsDisabled(ctx) {
		return nil, ModuleDisabledError("broadcasts")
	}
	b := &entity.Broadcast{
		Title:       title,
		Body:        body,
		Audience:    audience,
		Status:      entity.BroadcastStatusSending,
		ScheduledAt: scheduledAt,
		CreatedBy:   adminID,
	}
	if scheduledAt != nil {
		b.Status = entity.BroadcastStatusScheduled
		if err := uc.broadcasts.Create(ctx, b); err != nil {
			log.Printf("[AdminBroadcastUsecase.CreateBroadcast] create scheduled error: %v", err)
			return nil, err
		}
		return b, nil
	}
	if err := uc.broadcasts.Create(ctx, b); err != nil {
		log.Printf("[AdminBroadcastUsecase.CreateBroadcast] create instant error: %v", err)
		return nil, err
	}
	// F5: a failed instant fan-out must not strand the row as `sending`
	// with no retry: requeue to scheduled (due immediately) so the
	// 60s poller redelivers, then surface the error to the caller.
	// Duplicate POSTs are NOT deduplicated — each compose creates a new
	// blast, so clients must guard double-submit (disable the confirm
	// button while busy, as the admin UI does).
	sent, err := uc.deliver(ctx, b)
	if err != nil {
		log.Printf("[AdminBroadcastUsecase.CreateBroadcast] deliver error: %v", err)
		if rerr := uc.broadcasts.Requeue(ctx, b.ID); rerr != nil {
			log.Printf("[AdminBroadcastUsecase.CreateBroadcast] requeue broadcast %d error: %v", b.ID, rerr)
		}
		return nil, err
	}
	return sent, nil
}

// ListBroadcasts serves the admin broadcast history (newest first).
func (uc *AdminBroadcastUsecase) ListBroadcasts(ctx context.Context, page, limit int) ([]*entity.Broadcast, int64, error) {
	rows, total, err := uc.broadcasts.List(ctx, page, limit)
	if err != nil {
		log.Printf("[AdminBroadcastUsecase.ListBroadcasts] list error: %v", err)
		return nil, 0, err
	}
	return rows, total, nil
}

// CancelBroadcast moves a scheduled broadcast to cancelled. Anything else
// (sent, already cancelled, missing) is 404/422 and never delivers.
func (uc *AdminBroadcastUsecase) CancelBroadcast(ctx context.Context, id uint) (*entity.Broadcast, error) {
	b, err := uc.broadcasts.Cancel(ctx, id)
	if err != nil {
		log.Printf("[AdminBroadcastUsecase.CancelBroadcast] cancel error: %v", err)
		return nil, err
	}
	return b, nil
}

// EstimateRecipients counts active recipients for the compose confirm modal.
func (uc *AdminBroadcastUsecase) EstimateRecipients(ctx context.Context, audience string) (int64, error) {
	switch strings.TrimSpace(audience) {
	case entity.BroadcastAudienceAll, entity.BroadcastAudienceCustomer, entity.BroadcastAudienceProvider:
	default:
		return 0, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "audience must be all, customer, or provider")
	}
	total, err := uc.broadcasts.CountRecipients(ctx, strings.TrimSpace(audience))
	if err != nil {
		log.Printf("[AdminBroadcastUsecase.EstimateRecipients] count error: %v", err)
		return 0, err
	}
	return total, nil
}

// SendDueBroadcasts claims due scheduled rows and delivers each. One bad
// blast never stops the batch; failed rows are requeued for the next tick.
func (uc *AdminBroadcastUsecase) SendDueBroadcasts(ctx context.Context) error {
	// Ticket 12: while broadcasts are frozen, scheduled rows stay queued
	// (never delivered, never dropped) until the freeze lifts.
	if uc.broadcastsDisabled(ctx) {
		log.Printf("[AdminBroadcastUsecase.SendDueBroadcasts] broadcasts frozen, skipping tick")
		return nil
	}
	due, err := uc.broadcasts.ClaimDue(ctx, time.Now(), dueClaimLimit)
	if err != nil {
		log.Printf("[AdminBroadcastUsecase.SendDueBroadcasts] claim error: %v", err)
		return err
	}
	for _, b := range due {
		if _, err := uc.deliver(ctx, b); err != nil {
			log.Printf("[AdminBroadcastUsecase.SendDueBroadcasts] deliver broadcast %d error: %v", b.ID, err)
			if rerr := uc.broadcasts.Requeue(ctx, b.ID); rerr != nil {
				log.Printf("[AdminBroadcastUsecase.SendDueBroadcasts] requeue broadcast %d error: %v", b.ID, rerr)
			}
		}
	}
	return nil
}

// deliver fans a claimed (sending) broadcast out to every active recipient
// in its audience, one keyset-paged batch at a time, then flips the row to
// sent with the recipient snapshot.
func (uc *AdminBroadcastUsecase) deliver(ctx context.Context, b *entity.Broadcast) (*entity.Broadcast, error) {
	var total int64
	var afterID uint
	for {
		ids, err := uc.broadcasts.ListRecipientIDs(ctx, b.Audience, afterID, recipientPageSize)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		rows := make([]*entity.Notification, len(ids))
		for i, id := range ids {
			rows[i] = &entity.Notification{
				UserID: id,
				Type:   BroadcastNotificationType,
				Title:  b.Title,
				Body:   b.Body,
			}
		}
		if err := uc.notifications.CreateMany(ctx, rows); err != nil {
			return nil, err
		}
		total += int64(len(ids))
		afterID = ids[len(ids)-1]
		if len(ids) < recipientPageSize {
			break
		}
	}
	sent, err := uc.broadcasts.MarkSent(ctx, b.ID, total)
	if err != nil {
		return nil, err
	}
	return sent, nil
}

// StartBroadcastDuePoller fires due scheduled broadcasts every interval
// (60s in main.go), plus once at startup so a restart still delivers blasts
// whose time passed while the server was down. Stops when ctx is done;
// every goroutine here has this explicit shutdown path.
func StartBroadcastDuePoller(ctx context.Context, uc *AdminBroadcastUsecase, interval time.Duration) {
	run := func() {
		tctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()
		if err := uc.SendDueBroadcasts(tctx); err != nil {
			log.Printf("[BroadcastDuePoller] send due error: %v", err)
		}
	}
	go func() {
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
