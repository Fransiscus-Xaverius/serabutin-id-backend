package validator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/serabutin/serabutin-id-backend/pkg/whatsapp"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	if err := validate.RegisterValidation("whatsapp", func(fl validator.FieldLevel) bool {
		return whatsapp.Validate(fl.Field().String())
	}); err != nil {
		panic("failed to register whatsapp validation: " + err.Error())
	}
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
