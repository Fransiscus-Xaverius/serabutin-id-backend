package validator

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/serabutin/serabutin-id-backend/pkg/whatsapp"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
		return strings.ToLower(field.Name)
	})
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
			Field:   err.Field(),
			Message: validationMessage(err),
		})
	}

	return errors
}

func validationMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "is required"
	case "min":
		if isCollection(err) {
			return fmt.Sprintf("must contain at least %s", validationLimit(err))
		}
		return fmt.Sprintf("must be at least %s", validationLimit(err))
	case "max":
		if isCollection(err) {
			return fmt.Sprintf("must contain at most %s", validationLimit(err))
		}
		return fmt.Sprintf("must be at most %s", validationLimit(err))
	case "oneof":
		return fmt.Sprintf("must be one of: %s", strings.ReplaceAll(err.Param(), " ", ", "))
	case "email":
		return "must be a valid email address"
	case "http_url":
		return "must be a valid HTTP or HTTPS URL"
	case "whatsapp":
		return "must be a valid WhatsApp number"
	default:
		return fmt.Sprintf("is invalid (%s)", err.Tag())
	}
}

func isCollection(err validator.FieldError) bool {
	switch err.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

func validationLimit(err validator.FieldError) string {
	switch err.Kind() {
	case reflect.String:
		return fmt.Sprintf("%s characters", err.Param())
	case reflect.Array, reflect.Map, reflect.Slice:
		unit := "items"
		if err.Param() == "1" {
			unit = "item"
		}
		return fmt.Sprintf("%s %s", err.Param(), unit)
	default:
		return err.Param()
	}
}
