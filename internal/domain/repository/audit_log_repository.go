package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type AuditLogRepository interface {
	Create(ctx context.Context, entry *entity.AuditLog) error
	List(ctx context.Context, filter AuditLogFilter) ([]*entity.AuditLog, int64, error)
}

type AuditLogFilter struct {
	ActorUserID uint
	Action      string
	Page        int
	Limit       int
}
