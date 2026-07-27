package dto

import "errors"

type UpdateProfileRequest struct {
	Name         string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Bio          string `json:"bio,omitempty" validate:"omitempty,max=500"`
	Address      string `json:"address,omitempty" validate:"omitempty,max=255"`
	City         string `json:"city,omitempty" validate:"omitempty,max=100"`
	Province     string `json:"province,omitempty" validate:"omitempty,max=100"`
	Skills       string `json:"skills,omitempty" validate:"omitempty,max=500"`
	ServiceAreas string `json:"service_areas,omitempty" validate:"omitempty,max=500"`
	// Auto-accept budget range (spec 002). Either may be nil; if both
	// are set, min must be <= max.
	BudgetMin *int64 `json:"budget_min,omitempty" validate:"omitempty,min=0"`
	BudgetMax *int64 `json:"budget_max,omitempty" validate:"omitempty,min=0"`
}

// Validate enforces budget_min <= budget_max when both are set.
func (r UpdateProfileRequest) Validate() error {
	if r.BudgetMin != nil && r.BudgetMax != nil && *r.BudgetMin > *r.BudgetMax {
		return errors.New("budget_min cannot exceed budget_max")
	}
	return nil
}