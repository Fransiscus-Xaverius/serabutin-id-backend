package dto

type UpdateProfileRequest struct {
	Name         string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Bio          string `json:"bio,omitempty" validate:"omitempty,max=500"`
	Address      string `json:"address,omitempty" validate:"omitempty,max=255"`
	City         string `json:"city,omitempty" validate:"omitempty,max=100"`
	Province     string `json:"province,omitempty" validate:"omitempty,max=100"`
	Skills       string `json:"skills,omitempty" validate:"omitempty,max=500"`
	ServiceAreas string `json:"service_areas,omitempty" validate:"omitempty,max=500"`
}
