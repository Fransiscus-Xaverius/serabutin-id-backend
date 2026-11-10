package dto

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// TDD guard for spec 002: profile DTO must enforce budget_min <= budget_max
// when both are provided; either may be set independently; zero values
// (unset) are valid.
func TestUpdateProfileRequest_BudgetRangeValidation(t *testing.T) {
	v := validator.New()
	_ = v.RegisterValidation("budget_range", func(fl validator.FieldLevel) bool {
		// Cross-field validator: the struct has BudgetMin/BudgetMax.
		// We don't have direct access here, so the rule is enforced by
		// the struct-level Validate method below.
		return true
	})

	cases := []struct {
		name    string
		req     UpdateProfileRequest
		wantErr bool
	}{
		{"both unset", UpdateProfileRequest{}, false},
		{"min only", UpdateProfileRequest{BudgetMin: int64Ptr(10000)}, false},
		{"max only", UpdateProfileRequest{BudgetMax: int64Ptr(20000)}, false},
		{"min < max", UpdateProfileRequest{BudgetMin: int64Ptr(10000), BudgetMax: int64Ptr(20000)}, false},
		{"min == max", UpdateProfileRequest{BudgetMin: int64Ptr(15000), BudgetMax: int64Ptr(15000)}, false},
		{"min > max", UpdateProfileRequest{BudgetMin: int64Ptr(20000), BudgetMax: int64Ptr(10000)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.req.Validate()
			got := err != nil
			if got != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func int64Ptr(v int64) *int64 { return &v }
