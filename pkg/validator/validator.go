package validator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func Validate(s interface{}) []ValidationError {
	var errors []ValidationError

	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	for _, err := range err.(validator.ValidationErrors) {
		errors = append(errors, ValidationError{
			Field:   strings.ToLower(err.Field()),
			Message: fmt.Sprintf("validation failed on '%s' tag", err.Tag()),
		})
	}

	return errors
}
