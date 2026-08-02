package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type ProfileUsecase struct {
	userRepo repository.UserRepository
}

func NewProfileUsecase(userRepo repository.UserRepository) *ProfileUsecase {
	return &ProfileUsecase{userRepo: userRepo}
}

func (uc *ProfileUsecase) GetProfile(ctx context.Context, userID uint) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Printf("[ProfileUsecase.GetProfile] find user error: %v", err)
		return nil, err
	}
	return user, nil
}

func (uc *ProfileUsecase) UpdateProfile(ctx context.Context, userID uint, req dto.UpdateProfileRequest) (*entity.User, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Printf("[ProfileUsecase.UpdateProfile] find user error: %v", err)
		return nil, err
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Bio != "" {
		user.Bio = req.Bio
	}
	if req.Address != "" {
		user.Address = req.Address
	}
	if req.City != "" {
		user.City = req.City
	}
	if req.Province != "" {
		user.Province = req.Province
	}
	if req.Skills != "" {
		user.Skills = req.Skills
	}
	if req.ServiceAreas != "" {
		user.ServiceAreas = req.ServiceAreas
	}
	// Budget range may be explicitly cleared: nil pointer means "no change",
	// but a present pointer (even with value 0) means "set to this".
	if req.BudgetMin != nil {
		user.BudgetMin = req.BudgetMin
	}
	if req.BudgetMax != nil {
		user.BudgetMax = req.BudgetMax
	}

	if err := uc.userRepo.Update(ctx, user); err != nil {
		log.Printf("[ProfileUsecase.UpdateProfile] update user error: %v", err)
		return nil, err
	}

	return user, nil
}
