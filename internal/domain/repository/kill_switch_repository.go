package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// KillSwitchRepository persists kill-switch intent rows. The DB is the
// durable source: admin writes land here first, boot reloads Redis from
// here, and hot-path reads fall back to here on a Redis miss.
type KillSwitchRepository interface {
	List(ctx context.Context) ([]*entity.KillSwitch, error)
	// Get returns ErrNotFound when the module has no row yet.
	Get(ctx context.Context, module string) (*entity.KillSwitch, error)
	Upsert(ctx context.Context, sw *entity.KillSwitch) error
}
