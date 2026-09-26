package usecase

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

type NotificationUsecase struct {
	repo repository.NotificationRepository
}

// Notifier is the minimal interface booking/bid usecases depend on for
// fan-out. NotificationUsecase implements it; tests can stub it or leave
// it nil (notification is best-effort and never fails a transition).
type Notifier interface {
	Notify(ctx context.Context, userID uint, typ, title, body, data string) (*entity.Notification, error)
}

func NewNotificationUsecase(repo repository.NotificationRepository) *NotificationUsecase {
	return &NotificationUsecase{repo: repo}
}

func (uc *NotificationUsecase) Notify(ctx context.Context, userID uint, typ, title, body, data string) (*entity.Notification, error) {
	n := &entity.Notification{
		UserID: userID,
		Type:   typ,
		Title:  title,
		Body:   body,
		Data:   data,
	}
	if err := uc.repo.Create(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (uc *NotificationUsecase) List(ctx context.Context, userID uint, page, limit int) ([]*entity.Notification, int64, error) {
	return uc.repo.ListByUserID(ctx, userID, page, limit)
}

func (uc *NotificationUsecase) ListSince(ctx context.Context, userID uint, sinceID uint, limit int) ([]*entity.Notification, error) {
	return uc.repo.ListSinceID(ctx, userID, sinceID, limit)
}

func (uc *NotificationUsecase) ListAll(ctx context.Context, page, limit int, notifType string) ([]*entity.Notification, int64, error) {
	return uc.repo.ListAll(ctx, page, limit, notifType)
}

func (uc *NotificationUsecase) MarkRead(ctx context.Context, userID uint, id uint) error {
	return uc.repo.MarkRead(ctx, userID, id)
}
