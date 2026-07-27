package s3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePublicURL_RequiresAbsoluteHTTPURL(t *testing.T) {
	for _, prefix := range []string{"", "/minio", "minio", "ftp://example.com/minio", "//example.com/minio"} {
		t.Run(prefix, func(t *testing.T) {
			_, err := parsePublicURL(prefix)
			require.Error(t, err)
		})
	}
}

func TestJoinPublicURL_PreservesBucketAndNormalizesSlashes(t *testing.T) {
	base, err := parsePublicURL("https://objects.example.com/public/")
	require.NoError(t, err)

	got, err := joinPublicURL(base, "serabutin-uploads", "services/photo one.jpg")
	require.NoError(t, err)
	assert.Equal(t, "https://objects.example.com/public/serabutin-uploads/services/photo%20one.jpg", got)
}

func TestJoinPublicURL_RejectsUnsafeKey(t *testing.T) {
	base, err := parsePublicURL("https://objects.example.com")
	require.NoError(t, err)

	_, err = joinPublicURL(base, "bucket", "../secret")
	require.Error(t, err)
}

func TestS3Storage_ResolveImageURL(t *testing.T) {
	base, err := parsePublicURL("https://objects.example.com/public")
	require.NoError(t, err)
	storage := &S3Storage{bucket: "serabutin-uploads", urlPrefix: base}

	tests := map[string]string{
		"/uploads/legacy.jpg":              "https://objects.example.com/public/serabutin-uploads/legacy.jpg",
		"uploads/legacy.jpg":               "https://objects.example.com/public/serabutin-uploads/legacy.jpg",
		"https://cdn.example.com/a.jpg":    "https://cdn.example.com/a.jpg",
		"http://cdn.example.com/a.jpg":     "http://cdn.example.com/a.jpg",
		"data:image/png;base64,abc":        "data:image/png;base64,abc",
		"blob:http://localhost/identifier": "blob:http://localhost/identifier",
		"other/path.jpg":                   "other/path.jpg",
	}
	for input, want := range tests {
		assert.Equal(t, want, storage.ResolveImageURL(input), input)
	}
}
