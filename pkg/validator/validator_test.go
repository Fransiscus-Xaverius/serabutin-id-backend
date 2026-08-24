package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=8"`
	Name     string `validate:"required,min=2,max=100"`
}

func TestValidate_ValidStruct_ReturnsNil(t *testing.T) {
	t.Parallel()

	s := testStruct{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "John",
	}

	errors := Validate(s)
	assert.Nil(t, errors)
}

func TestValidate_InvalidEmail_ReturnsFieldError(t *testing.T) {
	t.Parallel()

	s := testStruct{
		Email:    "not-an-email",
		Password: "password123",
		Name:     "John",
	}

	errors := Validate(s)
	require.Len(t, errors, 1)
	assert.Equal(t, "email", errors[0].Field)
	assert.Contains(t, errors[0].Message, "email")
}

func TestValidate_RequiredField_ReturnsFieldError(t *testing.T) {
	t.Parallel()

	s := testStruct{
		Password: "password123",
		Name:     "John",
	}

	errors := Validate(s)
	require.Len(t, errors, 1)
	assert.Equal(t, "email", errors[0].Field)
	assert.Equal(t, "is required", errors[0].Message)
}

func TestValidate_MinLength_ReturnsFieldError(t *testing.T) {
	t.Parallel()

	s := testStruct{
		Email:    "user@example.com",
		Password: "short",
		Name:     "John",
	}

	errors := Validate(s)
	require.Len(t, errors, 1)
	assert.Equal(t, "password", errors[0].Field)
	assert.Equal(t, "must be at least 8 characters", errors[0].Message)
}

func TestValidate_MultipleErrors_ReturnsAllErrors(t *testing.T) {
	t.Parallel()

	s := testStruct{
		Email:    "not-an-email",
		Password: "short",
		Name:     "J",
	}

	errors := Validate(s)
	assert.Len(t, errors, 3)

	fieldMap := make(map[string]string)
	for _, e := range errors {
		fieldMap[e.Field] = e.Message
	}

	assert.Contains(t, fieldMap, "email")
	assert.Contains(t, fieldMap, "password")
	assert.Contains(t, fieldMap, "name")
}
