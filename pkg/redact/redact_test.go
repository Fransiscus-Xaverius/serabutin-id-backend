package redact

import (
	"testing"

	"github.com/stretchr/testify/require"
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
		{"empty_local", "@domain.com", "***@domain.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, RedactEmail(tt.email))
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
		{"min_redacted", "08123456789", "08******6789"},
		{"boundary_10", "0812345678", "08******5678"},
		{"boundary_9", "081234567", "08******4567"},
		{"boundary_8", "08123456", "***"},
		{"boundary_7", "0812345", "***"},
		{"boundary_6", "081234", "***"},
		{"short", "08123", "***"},
		{"empty", "", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, RedactPhone(tt.phone))
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
		{"extra_whitespace", "  John  Doe  ", "J*** D***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, RedactName(tt.input))
		})
	}
}
