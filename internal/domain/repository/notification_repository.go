package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type NotificationRepository interface {
	Create(ctx context.Context, n *entity.Notification) error
	ListByUserID(ctx context.Context, userID uint, page, limit int) ([]*entity.Notification, int64, error)
	ListSinceID(ctx context.Context, userID uint, sinceID uint, limit int) ([]*entity.Notification, error)
	ListAll(ctx context.Context, page, limit int, notifType string) ([]*entity.Notification, int64, error)
	MarkRead(ctx context.Context, userID uint, id uint) error
}
