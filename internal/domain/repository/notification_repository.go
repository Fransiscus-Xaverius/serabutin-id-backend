package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type NotificationRepository interface {
	Create(ctx context.Context, n *entity.Notification) error
	// CreateMany persists a fan-out batch in chunks of 500 rows
	// (broadcast blasts). Callers paginate recipients; this method never
	// loads more than one chunk at a time.
	CreateMany(ctx context.Context, notifications []*entity.Notification) error
	ListByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Notification, int64, error)
	ListSinceID(ctx context.Context, userID uint, sinceID uint, limit int) ([]*entity.Notification, error)
	ListAll(ctx context.Context, page, limit int, notifType string) ([]*entity.Notification, int64, error)
	MarkRead(ctx context.Context, userID uint, id uint) error
}
