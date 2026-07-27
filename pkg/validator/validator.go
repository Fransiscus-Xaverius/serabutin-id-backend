package validator

import (
	"fmt"
	"net/url"
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
	if err := validate.RegisterValidation("http_url", func(fl validator.FieldLevel) bool {
		parsed, err := url.Parse(fl.Field().String())
		if err != nil || parsed.Host == "" || parsed.User != nil {
			return false
		}
		return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
	}); err != nil {
		panic("failed to register HTTP URL validation: " + err.Error())
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
