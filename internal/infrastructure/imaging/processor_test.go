package imaging

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

func TestProcessor_Process(t *testing.T) {
	p := NewProcessor()
	testData := createTestImage(1000, 800)

	opts := service.ProcessOptions{
		Variants: []service.VariantConfig{
			{Name: "thumbnail", MaxWidth: 200, MaxHeight: 200},
			{Name: "medium", MaxWidth: 800, MaxHeight: 800},
			{Name: "original", MaxWidth: 0, MaxHeight: 0},
		},
		Quality: 85,
	}

	results, err := p.Process(testData, opts)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	for _, r := range results {
		assert.NotEmpty(t, r.Name)
		assert.NotEmpty(t, r.Data)
		assert.Equal(t, "image/jpeg", r.ContentType)
		assert.Greater(t, r.Width, 0)
		assert.Greater(t, r.Height, 0)
	}

	thumb := results[0]
	assert.Equal(t, "thumbnail", thumb.Name)
	assert.LessOrEqual(t, thumb.Width, 200)
	assert.LessOrEqual(t, thumb.Height, 200)

	medium := results[1]
	assert.Equal(t, "medium", medium.Name)
	assert.LessOrEqual(t, medium.Width, 800)

	original := results[2]
	assert.Equal(t, "original", original.Name)
	assert.Equal(t, 1000, original.Width)
	assert.Equal(t, 800, original.Height)
}

func TestProcessor_Process_AllVariantsGenerated(t *testing.T) {
	p := NewProcessor()
	testData := createTestImage(500, 500)

	opts := service.ProcessOptions{
		Variants: []service.VariantConfig{
			{Name: "small", MaxWidth: 100},
			{Name: "large", MaxWidth: 400},
		},
		Quality: 85,
	}

	results, err := p.Process(testData, opts)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
	}
	assert.True(t, names["small"])
	assert.True(t, names["large"])
}

func TestProcessor_Process_QualitySetting(t *testing.T) {
	p := NewProcessor()
	testData := createTestImage(500, 500)

	opts := service.ProcessOptions{
		Variants: []service.VariantConfig{
			{Name: "original"},
		},
		Quality: 50,
	}

	results, err := p.Process(testData, opts)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.NotEmpty(t, results[0].Data)
}

func TestProcessor_Process_InvalidImage(t *testing.T) {
	p := NewProcessor()

	opts := service.ProcessOptions{
		Variants: []service.VariantConfig{
			{Name: "original"},
		},
		Quality: 85,
	}

	_, err := p.Process([]byte("not an image"), opts)
	assert.Error(t, err)
}
