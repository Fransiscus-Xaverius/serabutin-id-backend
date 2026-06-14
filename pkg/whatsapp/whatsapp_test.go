package whatsapp

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"domestic format", "087812345678", "+6287812345678"},
		{"international without plus", "6287812345678", "+6287812345678"},
		{"international with plus", "+6287812345678", "+6287812345678"},
		{"with dashes", "0878-1234-5678", "+6287812345678"},
		{"with spaces", "0878 1234 5678", "+6287812345678"},
		{"with leading spaces", "  087812345678  ", "+6287812345678"},
		{"short number", "08123456789", "+628123456789"},
		{"long number", "0812345678901", "+62812345678901"},
		{"62 prefix with dashes", "62878-1234-5678", "+6287812345678"},
		{"+62 prefix with spaces", "+62 878 1234 5678", "+6287812345678"},
		{"unknown prefix", "12345678", "12345678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if result != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid domestic", "087812345678", true},
		{"valid international no plus", "6287812345678", true},
		{"valid international with plus", "+6287812345678", true},
		{"valid short", "08123456789", true},
		{"valid long", "0812345678901", true},
		{"with dashes", "0878-1234-5678", true},
		{"with spaces", "0878 1234 5678", true},
		{"invalid prefix 0", "097812345678", false},
		{"invalid prefix 00", "0087812345678", false},
		{"too short", "08781234", false},
		{"too long", "087812345678901", false},
		{"empty", "", false},
		{"non-numeric", "0878abcdefgh", false},
		{"wrong country code", "+187812345678", false},
		{"valid with spaces and dashes", "+62 878-1234-5678", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Validate(tt.input)
			if result != tt.expected {
				t.Errorf("Validate(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
