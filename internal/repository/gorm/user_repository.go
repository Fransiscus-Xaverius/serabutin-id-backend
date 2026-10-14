package gormrepo

import (
	"context"
	"errors"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	if user.Status == "" {
		user.Status = entity.UserStatusActive
	}
	m := &model.UserModel{
		Email:        user.Email,
		Password:     user.Password,
		Name:         user.Name,
		WhatsApp:     user.WhatsApp,
		Role:         user.Role,
		Status:       user.Status,
		Bio:          user.Bio,
		AvatarURL:    user.AvatarURL,
		Address:      user.Address,
		City:         user.City,
		Province:     user.Province,
		Skills:       user.Skills,
		ServiceAreas: user.ServiceAreas,
		BudgetMin:    user.BudgetMin,
		BudgetMax:    user.BudgetMax,
		ConsentAt:    user.ConsentAt,
	}

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperror.NewDomainError(apperror.ErrDuplicateEmail, "EMAIL_EXISTS", "email already registered")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	user.ID = m.ID
	user.CreatedAt = m.CreatedAt
	user.UpdatedAt = m.UpdatedAt

	return nil
}

func (r *UserRepository) Update(ctx context.Context, user *entity.User) error {
	m := &model.UserModel{
		ID:           user.ID,
		Email:        user.Email,
		Password:     user.Password,
		Name:         user.Name,
		WhatsApp:     user.WhatsApp,
		Role:         user.Role,
		Status:       user.Status,
		Bio:          user.Bio,
		AvatarURL:    user.AvatarURL,
		Address:      user.Address,
		City:         user.City,
		Province:     user.Province,
		Skills:       user.Skills,
		ServiceAreas: user.ServiceAreas,
		BudgetMin:    user.BudgetMin,
		BudgetMax:    user.BudgetMax,
		ConsentAt:    user.ConsentAt,
	}

	result := r.db.WithContext(ctx).Save(m)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return apperror.NewDomainError(apperror.ErrDuplicateEmail, "DUPLICATE", "duplicate entry")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}

	user.UpdatedAt = m.UpdatedAt

	return nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	var m model.UserModel

	err := r.db.WithContext(ctx).Where("email = ?", email).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *UserRepository) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	var m model.UserModel

	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *UserRepository) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	var m model.UserModel

	err := r.db.WithContext(ctx).Where("whats_app = ?", whatsapp).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&model.UserModel{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return count > 0, nil
}

func (r *UserRepository) toEntity(m *model.UserModel) *entity.User {
	return &entity.User{
		ID:           m.ID,
		Email:        m.Email,
		Password:     m.Password,
		Name:         m.Name,
		WhatsApp:     m.WhatsApp,
		Role:         m.Role,
		Status:       m.Status,
		Bio:          m.Bio,
		AvatarURL:    m.AvatarURL,
		Address:      m.Address,
		City:         m.City,
		Province:     m.Province,
		Skills:       m.Skills,
		ServiceAreas: m.ServiceAreas,
		BudgetMin:    m.BudgetMin,
		BudgetMax:    m.BudgetMax,
		ConsentAt:    m.ConsentAt,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
