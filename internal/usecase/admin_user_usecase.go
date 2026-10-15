package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// AdminUserUsecase owns admin user-directory mutations. Single-row writes go
// through the repository's column-scoped updates (no multi-repo transaction
// needed); every mutation is audit-logged by middleware.AdminAuditLogger, so
// the handler enforces a mandatory `reason` and this layer never writes audit
// rows directly.
type AdminUserUsecase struct {
	userRepo repository.UserRepository
	// tokenRepo revokes live sessions on Suspend (F1). Nil means ungated
	// (Authenticate still rejects suspended accounts on next request).
	tokenRepo repository.TokenRepository
}

func NewAdminUserUsecase(userRepo repository.UserRepository) *AdminUserUsecase {
	return &AdminUserUsecase{userRepo: userRepo}
}

// SetTokenRepository attaches the session store for Suspend-time revocation.
// Kept as a setter so the established constructor shape — relied on by
// existing usecase/handler tests — stays stable.
func (uc *AdminUserUsecase) SetTokenRepository(repo repository.TokenRepository) {
	uc.tokenRepo = repo
}

func (uc *AdminUserUsecase) ListUsers(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	users, total, err := uc.userRepo.List(ctx, filter)
	if err != nil {
		log.Printf("[AdminUserUsecase.ListUsers] list users error: %v", err)
		return nil, 0, err
	}
	return users, total, nil
}

func (uc *AdminUserUsecase) GetUser(ctx context.Context, id uint) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminUserUsecase.GetUser] find user error: %v", err)
		return nil, err
	}
	return user, nil
}

// Suspend sets status=suspended. Idempotent: an already-suspended user
// returns 200 without a write (the audit middleware still records the call).
// Guards (F8): admins cannot suspend themselves (SELF_SUSPEND → 403), and
// suspending an admin is refused while they are the last active admin
// (LAST_ADMIN → 403). On success every live token is revoked (F1); a
// revocation-store outage is logged but never fails the suspend —
// Authenticate rejects suspended accounts on next request regardless.
func (uc *AdminUserUsecase) Suspend(ctx context.Context, actorID, id uint) (*entity.User, error) {
	if actorID != 0 && actorID == id {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "SELF_SUSPEND", "admins cannot suspend themselves")
	}
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminUserUsecase.Suspend] find user error: %v", err)
		return nil, err
	}
	if user.Status == entity.UserStatusSuspended {
		return user, nil
	}
	if user.Role == "admin" {
		if err := uc.guardLastAdmin(ctx); err != nil {
			return nil, err
		}
	}
	if err := uc.userRepo.UpdateStatus(ctx, id, entity.UserStatusSuspended); err != nil {
		log.Printf("[AdminUserUsecase.Suspend] update status error: %v", err)
		return nil, err
	}
	user.Status = entity.UserStatusSuspended
	if uc.tokenRepo != nil {
		if err := uc.tokenRepo.RevokeAllUserTokens(ctx, id); err != nil {
			log.Printf("[AdminUserUsecase.Suspend] revoke tokens for user %d error: %v", id, err)
		}
	}
	return user, nil
}

// Restore sets status=active. Idempotent like Suspend.
func (uc *AdminUserUsecase) Restore(ctx context.Context, id uint) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminUserUsecase.Restore] find user error: %v", err)
		return nil, err
	}
	if user.Status == entity.UserStatusActive {
		return user, nil
	}
	if err := uc.userRepo.UpdateStatus(ctx, id, entity.UserStatusActive); err != nil {
		log.Printf("[AdminUserUsecase.Restore] update status error: %v", err)
		return nil, err
	}
	user.Status = entity.UserStatusActive
	return user, nil
}

// SetRole changes a user's role. Any existing admin may call it (route-level
// RequireAdmin guarantees the actor is an admin); promoting to admin grants
// full admin access and is audit-logged. An admin cannot demote themselves
// (SELF_DEMOTE → 403); demoting a fellow admin is refused while they are the
// last active admin (LAST_ADMIN → 403).
func (uc *AdminUserUsecase) SetRole(ctx context.Context, actorID, id uint, role string) (*entity.User, error) {
	switch role {
	case "customer", "provider", "admin":
	default:
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "role must be customer, provider, or admin")
	}
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminUserUsecase.SetRole] find user error: %v", err)
		return nil, err
	}
	if actorID == id && user.Role == "admin" && role != "admin" {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "SELF_DEMOTE", "admins cannot demote themselves")
	}
	if user.Role == role {
		return user, nil
	}
	if user.Role == "admin" && role != "admin" {
		if err := uc.guardLastAdmin(ctx); err != nil {
			return nil, err
		}
	}
	if err := uc.userRepo.UpdateRole(ctx, id, role); err != nil {
		log.Printf("[AdminUserUsecase.SetRole] update role error: %v", err)
		return nil, err
	}
	user.Role = role
	return user, nil
}

// guardLastAdmin refuses admin-targeting suspend/demote while the target
// would be the last active admin. The count includes the target (still
// active at check time), so <= 1 means no other admin survives.
func (uc *AdminUserUsecase) guardLastAdmin(ctx context.Context) error {
	n, err := uc.userRepo.CountActiveAdmins(ctx)
	if err != nil {
		log.Printf("[AdminUserUsecase.guardLastAdmin] count error: %v", err)
		return err
	}
	if n <= 1 {
		return apperror.NewDomainError(apperror.ErrForbidden, "LAST_ADMIN", "cannot remove the last active admin")
	}
	return nil
}
