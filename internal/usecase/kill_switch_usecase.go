package usecase

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// KillSwitchRedisPrefix namespaces runtime keys: killswitch:<module> ("1"/"0", no TTL).
const KillSwitchRedisPrefix = "killswitch:"

// KillSwitch master confirm guards against fat-finger freezes.
const (
	KillSwitchConfirmFreeze   = "FREEZE"
	KillSwitchConfirmUnfreeze = "UNFREEZE"
)

// KillSwitchCache is the minimal Redis surface the kill-switch needs —
// kept as an interface so hot-path reads stay unit-testable without Redis.
type KillSwitchCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// RedisKillSwitchCache adapts *redis.Client to KillSwitchCache.
type RedisKillSwitchCache struct {
	client *redis.Client
}

func NewRedisKillSwitchCache(client *redis.Client) *RedisKillSwitchCache {
	return &RedisKillSwitchCache{client: client}
}

func (r *RedisKillSwitchCache) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisKillSwitchCache) Set(ctx context.Context, key, value string) error {
	return r.client.Set(ctx, key, value, 0).Err()
}

// KillSwitchUsecase owns the hybrid kill-switch store: DB is the durable
// intent (admin writes land here first), Redis is the runtime source
// (hot-path reads hit it with DB fallback on miss). Writes publish to Redis
// best-effort AFTER the DB commit, so a Redis outage never loses intent.
// Reads fail OPEN: when both Redis and DB are unreachable the module is
// treated as enabled (with a logged warning), so an infra outage cannot
// freeze the platform. See decisions/005 for the rationale.
type KillSwitchUsecase struct {
	repo  repository.KillSwitchRepository
	cache KillSwitchCache // nil means DB-only (tests, Redis-less setups)
}

func NewKillSwitchUsecase(repo repository.KillSwitchRepository, cache KillSwitchCache) *KillSwitchUsecase {
	return &KillSwitchUsecase{repo: repo, cache: cache}
}

func killSwitchKey(module string) string {
	return KillSwitchRedisPrefix + module
}

func killSwitchRedisValue(enabled bool) string {
	if enabled {
		return "1"
	}
	return "0"
}

// List returns the durable DB rows (admin reads always see intent, never a
// stale or missing runtime key).
func (uc *KillSwitchUsecase) List(ctx context.Context) ([]*entity.KillSwitch, error) {
	rows, err := uc.repo.List(ctx)
	if err != nil {
		log.Printf("[KillSwitchUsecase.List] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

// SetEnabled validates and persists a toggle: DB upsert first, then
// best-effort Redis publish. A Redis publish failure is logged but never
// fails the write — the next read falls back to DB and boot re-syncs.
func (uc *KillSwitchUsecase) SetEnabled(ctx context.Context, adminID uint, module string, enabled bool, reason, confirm string) (*entity.KillSwitch, error) {
	module = strings.ToLower(strings.TrimSpace(module))
	if !entity.IsValidKillSwitchModule(module) {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "unknown module")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "reason is required")
	}
	if module == entity.KillSwitchMaster {
		want := KillSwitchConfirmFreeze
		if enabled {
			want = KillSwitchConfirmUnfreeze
		}
		if confirm != want {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "confirm must be '"+want+"'")
		}
	}
	sw := &entity.KillSwitch{
		Module:    module,
		Enabled:   enabled,
		UpdatedBy: adminID,
		Reason:    strings.TrimSpace(reason),
	}
	if err := uc.repo.Upsert(ctx, sw); err != nil {
		log.Printf("[KillSwitchUsecase.SetEnabled] db upsert %s error: %v", module, err)
		return nil, err
	}
	if uc.cache != nil {
		if err := uc.cache.Set(ctx, killSwitchKey(module), killSwitchRedisValue(enabled)); err != nil {
			log.Printf("[KillSwitchUsecase.SetEnabled] redis publish %s error (DB already committed): %v", module, err)
		}
	}
	return sw, nil
}

// IsEnabled reports whether a product module may serve writes. Master off
// freezes every module (OR semantics). Unknown state fails open (true).
func (uc *KillSwitchUsecase) IsEnabled(ctx context.Context, module string) bool {
	if enabled, known := uc.runtimeEnabled(ctx, entity.KillSwitchMaster); known && !enabled {
		return false
	}
	if module == entity.KillSwitchMaster {
		return true
	}
	if enabled, known := uc.runtimeEnabled(ctx, module); known && !enabled {
		return false
	}
	return true
}

// ChatDisabled is the chat-backend signal served via introspect: true when
// the chat module (or the master freeze) disables chat.
func (uc *KillSwitchUsecase) ChatDisabled(ctx context.Context) bool {
	return !uc.IsEnabled(ctx, entity.KillSwitchChat)
}

// runtimeEnabled reads one flag from Redis with DB fallback. The second
// return is false when neither source could answer (fail-open upstream).
func (uc *KillSwitchUsecase) runtimeEnabled(ctx context.Context, module string) (enabled, known bool) {
	if uc.cache != nil {
		raw, err := uc.cache.Get(ctx, killSwitchKey(module))
		if err == nil {
			return raw == "1", true
		}
		if !errors.Is(err, redis.Nil) {
			log.Printf("[KillSwitchUsecase] redis read %s failed, falling back to DB: %v", module, err)
		}
	}
	sw, err := uc.repo.Get(ctx, module)
	if err != nil {
		if !errors.Is(err, apperror.ErrNotFound) {
			log.Printf("[KillSwitchUsecase] db read %s failed, failing open: %v", module, err)
		}
		// No row yet (pre-seed) or DB down: unknown, caller fails open.
		return true, false
	}
	return sw.Enabled, true
}

// EnsureSeeded writes all-enabled rows for modules missing from the DB.
// Idempotent: existing rows are never overwritten.
func (uc *KillSwitchUsecase) EnsureSeeded(ctx context.Context) error {
	for _, m := range entity.KillSwitchModules {
		_, err := uc.repo.Get(ctx, m)
		if err == nil {
			continue
		}
		if !errors.Is(err, apperror.ErrNotFound) {
			return err
		}
		if err := uc.repo.Upsert(ctx, &entity.KillSwitch{Module: m, Enabled: true, Reason: "default seed"}); err != nil {
			return err
		}
	}
	return nil
}

// SyncToRedis reloads every runtime key from the DB. Called on boot so a
// Redis flush never loses intent. Returns the first publish error after
// attempting all keys.
func (uc *KillSwitchUsecase) SyncToRedis(ctx context.Context) error {
	if uc.cache == nil {
		return nil
	}
	rows, err := uc.repo.List(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, sw := range rows {
		if err := uc.cache.Set(ctx, killSwitchKey(sw.Module), killSwitchRedisValue(sw.Enabled)); err != nil {
			log.Printf("[KillSwitchUsecase.SyncToRedis] publish %s error: %v", sw.Module, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ModuleDisabledError builds the 503-domain error enforcement points return
// when a write hits a disabled module. apierror maps the MODULE_DISABLED
// code to HTTP 503 with the module named in the message.
func ModuleDisabledError(module string) *apperror.DomainError {
	return apperror.NewDomainError(apperror.ErrModuleDisabled, "MODULE_DISABLED", "module '"+module+"' is temporarily disabled")
}
