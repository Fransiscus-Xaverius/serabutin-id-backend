package seed

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/stretchr/testify/assert"
)

func TestGenerateServices_ReturnsCorrectCount(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	assert.Len(t, services, 1000)
}

func TestGenerateServices_AllLinkedToProviders(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	providerIDs := make(map[uint]bool)
	for _, p := range providers {
		providerIDs[p.ID] = true
	}
	for _, s := range services {
		assert.True(t, providerIDs[s.ProviderID], "service %d has unknown provider %d", s.ID, s.ProviderID)
	}
}

func TestGenerateServices_HasImages(t *testing.T) {
	providers := make([]model.UserModel, 200)
	for i := range providers {
		providers[i].ID = uint(i + 1)
	}
	services := generateServices(providers)
	for _, s := range services {
		assert.GreaterOrEqual(t, len(s.Images), 2)
	}
}
