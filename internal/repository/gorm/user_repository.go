package gormrepo

import (
	"context"
	"errors"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
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
		ChatBlocked:  user.ChatBlocked,
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
	// created_at is immutable: reload the stored row and preserve it, so a
	// caller passing zero timestamps (or a stale copy) can never clobber it
	// via Save's full-column write.
	var existing model.UserModel
	if err := r.db.WithContext(ctx).First(&existing, user.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	m := &model.UserModel{
		ID:           user.ID,
		Email:        user.Email,
		Password:     user.Password,
		Name:         user.Name,
		WhatsApp:     user.WhatsApp,
		Role:         user.Role,
		Status:       user.Status,
		ChatBlocked:  user.ChatBlocked,
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
		CreatedAt:    existing.CreatedAt,
	}

	result := r.db.WithContext(ctx).Save(m)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return apperror.NewDomainError(apperror.ErrDuplicateEmail, "DUPLICATE", "duplicate entry")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}

	user.CreatedAt = m.CreatedAt
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

// List returns admin-directory rows: partial match on email/name/whatsapp via
// Q, exact matches on email/whatsapp/role/status, newest first, offset
// paginated with a separate total count.
func (r *UserRepository) List(ctx context.Context, filter repository.UserFilter) ([]*entity.User, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.UserModel{})
	if filter.Q != "" {
		like := "%" + filter.Q + "%"
		query = query.Where("email ILIKE ? OR name ILIKE ? OR whats_app LIKE ?", like, like, like)
	}
	if filter.Email != "" {
		query = query.Where("email = ?", filter.Email)
	}
	if filter.WhatsApp != "" {
		query = query.Where("whats_app = ?", filter.WhatsApp)
	}
	if filter.Role != "" {
		query = query.Where("role = ?", filter.Role)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var models []model.UserModel
	if err := query.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.User, len(models))
	for i := range models {
		out[i] = r.toEntity(&models[i])
	}
	return out, total, nil
}

// UpdateStatus writes only the status column.
func (r *UserRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	result := r.db.WithContext(ctx).Model(&model.UserModel{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return nil
}

// UpdateRole writes only the role column.
func (r *UserRepository) UpdateRole(ctx context.Context, id uint, role string) error {
	result := r.db.WithContext(ctx).Model(&model.UserModel{}).Where("id = ?", id).Update("role", role)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return nil
}

// UpdateChatBlocked writes only the chat_blocked column.
func (r *UserRepository) UpdateChatBlocked(ctx context.Context, id uint, blocked bool) error {
	result := r.db.WithContext(ctx).Model(&model.UserModel{}).Where("id = ?", id).Update("chat_blocked", blocked)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return nil
}

var _ repository.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) toEntity(m *model.UserModel) *entity.User {
	return &entity.User{
		ID:           m.ID,
		Email:        m.Email,
		Password:     m.Password,
		Name:         m.Name,
		WhatsApp:     m.WhatsApp,
		Role:         m.Role,
		Status:       m.Status,
		ChatBlocked:  m.ChatBlocked,
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
