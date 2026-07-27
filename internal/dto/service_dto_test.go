package dto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

func TestServiceImageRequest_URLValidation(t *testing.T) {
	validCreate := CreateServiceRequest{
		Title:           "Home Cleaning",
		Category:        "cleaning",
		Description:     "Complete home cleaning",
		UnitType:        "per_session",
		BasePrice:       10000,
		DurationMinutes: 60,
		ServiceAreas:    "Jakarta",
	}

	for _, tt := range []struct {
		name    string
		url     string
		invalid bool
	}{
		{"https", "https://media.example.com/uploads/photo.jpg", false},
		{"http", "http://localhost:3000/uploads/photo.jpg", false},
		{"relative", "/uploads/photo.jpg", true},
		{"data", "data:image/png;base64,abc", true},
		{"blob", "blob:http://localhost/id", true},
		{"ftp", "ftp://media.example.com/photo.jpg", true},
		{"over 500", "https://media.example.com/" + strings.Repeat("a", 500), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			create := validCreate
			create.Images = []ServiceImageRequest{{URL: tt.url}}
			update := UpdateServiceRequest{Images: []ServiceImageRequest{{URL: tt.url}}}

			assert.Equal(t, tt.invalid, validator.Validate(create) != nil)
			assert.Equal(t, tt.invalid, validator.Validate(update) != nil)
		})
	}
}
