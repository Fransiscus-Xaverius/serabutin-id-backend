package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type ReportRepository interface {
	Create(ctx context.Context, report *entity.Report) error
	FindByID(ctx context.Context, id uint) (*entity.Report, error)
	// ListForAdmin serves the admin report queue, newest first.
	ListForAdmin(ctx context.Context, filter ReportFilter) ([]*entity.Report, int64, error)
	// Resolve moves an open report to resolved|dismissed (terminal).
	Resolve(ctx context.Context, id uint, adminID uint, status string) error
}

// ReportFilter scopes the admin report queue. Empty fields mean no
// constraint on that axis.
type ReportFilter struct {
	Status     string
	TargetType string
	Page       int
	Limit      int
}
