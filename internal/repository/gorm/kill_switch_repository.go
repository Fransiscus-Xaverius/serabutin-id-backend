package gormrepo

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

// KillSwitchRepository is the GORM durable store for kill-switch intent.
type KillSwitchRepository struct {
	db *gorm.DB
}

func NewKillSwitchRepository(db *gorm.DB) *KillSwitchRepository {
	return &KillSwitchRepository{db: db}
}

func toKillSwitchEntity(m *model.KillSwitchModel) *entity.KillSwitch {
	return &entity.KillSwitch{
		Module:    m.Module,
		Enabled:   m.Enabled,
		UpdatedBy: m.UpdatedBy,
		UpdatedAt: m.UpdatedAt,
		Reason:    m.Reason,
	}
}

func (r *KillSwitchRepository) List(ctx context.Context) ([]*entity.KillSwitch, error) {
	var models []model.KillSwitchModel
	if err := r.db.WithContext(ctx).Model(&model.KillSwitchModel{}).Order("module ASC").Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.KillSwitch, len(models))
	for i := range models {
		out[i] = toKillSwitchEntity(&models[i])
	}
	return out, nil
}

func (r *KillSwitchRepository) Get(ctx context.Context, module string) (*entity.KillSwitch, error) {
	var m model.KillSwitchModel
	if err := r.db.WithContext(ctx).Where("module = ?", module).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "KILL_SWITCH_NOT_FOUND", "kill switch not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return toKillSwitchEntity(&m), nil
}

func (r *KillSwitchRepository) Upsert(ctx context.Context, sw *entity.KillSwitch) error {
	m := &model.KillSwitchModel{
		Module:    sw.Module,
		Enabled:   sw.Enabled,
		UpdatedBy: sw.UpdatedBy,
		Reason:    sw.Reason,
	}
	// Module is the primary key, so Save is an upsert by construction.
	if err := r.db.WithContext(ctx).Save(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	sw.UpdatedAt = m.UpdatedAt
	return nil
}

var _ repository.KillSwitchRepository = (*KillSwitchRepository)(nil)
