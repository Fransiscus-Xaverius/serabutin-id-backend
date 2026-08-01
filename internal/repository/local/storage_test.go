package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStorage_Upload(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "http://localhost:3000/uploads/")
	require.NoError(t, err)

	data := []byte("test file content")
	url, err := storage.Upload(context.Background(), "test/file.txt", data, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000/uploads/test/file.txt", url)

	content, err := os.ReadFile(filepath.Join(tmpDir, "test/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestLocalStorage_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "http://localhost:3000/uploads")
	require.NoError(t, err)

	data := []byte("test content")
	_, err = storage.Upload(context.Background(), "delete-me.txt", data, "text/plain")
	require.NoError(t, err)

	err = storage.Delete(context.Background(), "delete-me.txt")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "delete-me.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestLocalStorage_Delete_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "http://localhost:3000/uploads")
	require.NoError(t, err)

	err = storage.Delete(context.Background(), "non-existent.txt")
	assert.NoError(t, err)
}

func TestLocalStorage_GetURL(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "http://localhost:3000/uploads/")
	require.NoError(t, err)

	url := storage.GetURL("images/photo.jpg")
	assert.Equal(t, "http://localhost:3000/uploads/images/photo.jpg", url)
}

func TestLocalStorage_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "nested", "dir")

	storage, err := NewLocalStorage(newDir, "http://localhost:3000/uploads")
	require.NoError(t, err)
	assert.NotNil(t, storage)

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewLocalStorage_RequiresAbsoluteHTTPURL(t *testing.T) {
	for _, baseURL := range []string{"/uploads", "uploads", "https://cdn.example.com", "https://cdn.example.com/", "https://cdn.example.com/media%2Fuploads", "ftp://cdn.example.com/uploads", "//cdn.example.com/uploads"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := NewLocalStorage(t.TempDir(), baseURL)
			require.Error(t, err)
		})
	}
}

func TestLocalStorage_CanonicalizesPublicURLPath(t *testing.T) {
	for _, baseURL := range []string{
		"https://media.example.com/media//uploads",
		"https://media.example.com/public/../media/uploads",
	} {
		storage, err := NewLocalStorage(t.TempDir(), baseURL)
		require.NoError(t, err)
		assert.Equal(t, "https://media.example.com/media/uploads/images/photo.jpg", storage.GetURL("images/photo.jpg"))
	}
}

func TestLocalStorage_RejectsUnsafeKeys(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), "https://media.example.com/uploads")
	require.NoError(t, err)

	for _, key := range []string{"../secret.txt", "images/../../secret.txt", "/absolute.txt", `C:\\absolute.txt`, "https://example.com/file.jpg"} {
		t.Run(key, func(t *testing.T) {
			_, err := storage.Upload(context.Background(), key, []byte("secret"), "text/plain")
			require.Error(t, err)
			require.Error(t, storage.Delete(context.Background(), key))
		})
	}
}

func TestLocalStorage_GetURL_JoinsEscapedKey(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), "https://media.example.com/base/uploads/")
	require.NoError(t, err)

	assert.Equal(t, "https://media.example.com/base/uploads/folder/photo%20one.jpg", storage.GetURL("folder/photo one.jpg"))
}

func TestLocalStorage_ResolveImageURL(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), "https://media.example.com/uploads")
	require.NoError(t, err)

	tests := map[string]string{
		"/uploads/legacy.jpg":              "https://media.example.com/uploads/legacy.jpg",
		"uploads/legacy.jpg":               "https://media.example.com/uploads/legacy.jpg",
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
