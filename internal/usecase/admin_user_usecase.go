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
}

func NewAdminUserUsecase(userRepo repository.UserRepository) *AdminUserUsecase {
	return &AdminUserUsecase{userRepo: userRepo}
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
func (uc *AdminUserUsecase) Suspend(ctx context.Context, id uint) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminUserUsecase.Suspend] find user error: %v", err)
		return nil, err
	}
	if user.Status == entity.UserStatusSuspended {
		return user, nil
	}
	if err := uc.userRepo.UpdateStatus(ctx, id, entity.UserStatusSuspended); err != nil {
		log.Printf("[AdminUserUsecase.Suspend] update status error: %v", err)
		return nil, err
	}
	user.Status = entity.UserStatusSuspended
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
// full admin access and is audit-logged. An admin cannot demote themselves.
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
	if err := uc.userRepo.UpdateRole(ctx, id, role); err != nil {
		log.Printf("[AdminUserUsecase.SetRole] update role error: %v", err)
		return nil, err
	}
	user.Role = role
	return user, nil
}
