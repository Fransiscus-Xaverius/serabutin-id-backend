package usecase

import (
	"context"
	"fmt"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Admin moderation targets. Only these terminal states may be set by admins.
const (
	AdminServiceStatusActive    = "active"
	AdminServiceStatusRejected  = "rejected"
	AdminServiceStatusTakenDown = "taken_down"
)

// adminServiceTransitions is the moderation state machine:
//   - draft|pending -> active (approve) | rejected
//   - active -> taken_down | rejected
//   - rejected|taken_down -> active (appeal restore)
//
// Same-status calls are idempotent no-ops (handled by the caller, not here).
var adminServiceTransitions = map[string]map[string]bool{
	"draft":      {AdminServiceStatusActive: true, AdminServiceStatusRejected: true},
	"pending":    {AdminServiceStatusActive: true, AdminServiceStatusRejected: true},
	"active":     {AdminServiceStatusTakenDown: true, AdminServiceStatusRejected: true},
	"rejected":   {AdminServiceStatusActive: true},
	"taken_down": {AdminServiceStatusActive: true},
	// Legacy provider-side states have no moderation path except restore.
	"inactive": {AdminServiceStatusActive: true},
	"deleted":  {AdminServiceStatusActive: true},
}

// AdminServiceUsecase owns admin jasa-moderation reads and status mutations.
// Single-row status writes go through the repository's column-scoped update
// (no multi-repo transaction needed); every mutation is audit-logged by
// middleware.AdminAuditLogger, so the handler enforces a mandatory `reason`
// and this layer never writes audit rows directly.
type AdminServiceUsecase struct {
	serviceRepo repository.ServiceRepository
}

func NewAdminServiceUsecase(serviceRepo repository.ServiceRepository) *AdminServiceUsecase {
	return &AdminServiceUsecase{serviceRepo: serviceRepo}
}

func (uc *AdminServiceUsecase) ListServices(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	services, total, err := uc.serviceRepo.ListForAdmin(ctx, filter)
	if err != nil {
		log.Printf("[AdminServiceUsecase.ListServices] list services error: %v", err)
		return nil, 0, err
	}
	return services, total, nil
}

// GetService returns any service regardless of status (bypasses the public
// active-only gate) so rejected/taken-down listings stay inspectable.
func (uc *AdminServiceUsecase) GetService(ctx context.Context, id uint) (*entity.Service, error) {
	service, err := uc.serviceRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminServiceUsecase.GetService] find service error: %v", err)
		return nil, err
	}
	return service, nil
}

// SetServiceStatus applies one moderation transition. Unknown targets are
// rejected with VALIDATION_ERROR (400); disallowed transitions with
// INVALID_STATUS_TRANSITION (422). Setting the current status is an
// idempotent success without a write.
func (uc *AdminServiceUsecase) SetServiceStatus(ctx context.Context, id uint, status string) (*entity.Service, error) {
	switch status {
	case AdminServiceStatusActive, AdminServiceStatusRejected, AdminServiceStatusTakenDown:
	default:
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "status must be active, rejected, or taken_down")
	}
	service, err := uc.serviceRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[AdminServiceUsecase.SetServiceStatus] find service error: %v", err)
		return nil, err
	}
	if service.Status == status {
		return service, nil
	}
	allowed, ok := adminServiceTransitions[service.Status]
	if !ok {
		allowed = map[string]bool{AdminServiceStatusActive: true}
	}
	if !allowed[status] {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION",
			fmt.Sprintf("cannot transition service from %s to %s", service.Status, status))
	}
	if err := uc.serviceRepo.SetStatus(ctx, id, status); err != nil {
		log.Printf("[AdminServiceUsecase.SetServiceStatus] set status error: %v", err)
		return nil, err
	}
	service.Status = status
	return service, nil
}
