package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type UserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	Update(ctx context.Context, user *entity.User) error
	FindByEmail(ctx context.Context, email string) (*entity.User, error)
	FindByID(ctx context.Context, id uint) (*entity.User, error)
	FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	List(ctx context.Context, filter UserFilter) ([]*entity.User, int64, error)
	UpdateStatus(ctx context.Context, id uint, status string) error
	UpdateRole(ctx context.Context, id uint, role string) error
	// UpdateChatBlocked writes only the chat_blocked column (ticket 8).
	UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error
}

type UserFilter struct {
	Q        string
	Email    string
	WhatsApp string
	Role     string
	Status   string
	Page     int
	Limit    int
}
