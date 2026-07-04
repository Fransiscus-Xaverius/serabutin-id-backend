package seed

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomItemReturnsItemFromSlice(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	items := []string{"a", "b", "c"}
	got := randomItem[string](rng, items)
	assert.Contains(t, items, got)
}

func TestRandomIntWithinRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		got := randomInt(rng, 5, 10)
		assert.GreaterOrEqual(t, got, 5)
		assert.LessOrEqual(t, got, 10)
	}
}
