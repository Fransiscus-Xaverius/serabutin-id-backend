package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"standard", "john.doe@example.com", "j***@example.com"},
		{"short", "a@b.com", "a***@b.com"},
		{"empty", "", "***@"},
		{"no_at", "invalid", "***@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactEmail(tt.email))
		})
	}
}

func TestRedactPhone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		phone    string
		expected string
	}{
		{"standard", "081234567890", "08******7890"},
		{"short", "0812345678", "08******5678"},
		{"empty", "", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactPhone(tt.phone))
		})
	}
}

func TestRedactName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single", "John", "J***"},
		{"full", "John Doe", "J*** D***"},
		{"three", "John Paul Doe", "J*** P*** D***"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactName(tt.input))
		})
	}
}
