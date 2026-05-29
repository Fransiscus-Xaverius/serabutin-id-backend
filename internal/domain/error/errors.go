package apperror

import "errors"

var (
	ErrNotFound       = errors.New("resource not found")
	ErrDuplicateEmail = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTokenExpired    = errors.New("token has expired")
	ErrTokenRevoked    = errors.New("token has been revoked")
	ErrTokenInvalid    = errors.New("token is invalid")
	ErrInternalServer  = errors.New("internal server error")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrValidation      = errors.New("validation failed")
)

type DomainError struct {
	Err     error
	Message string
	Code    string
}

func (e *DomainError) Error() string {
	return e.Message
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

func NewDomainError(err error, code, message string) *DomainError {
	return &DomainError{
		Err:     err,
		Code:    code,
		Message: message,
	}
}
