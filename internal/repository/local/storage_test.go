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
	storage, err := NewLocalStorage(tmpDir, "/uploads")
	require.NoError(t, err)

	data := []byte("test file content")
	url, err := storage.Upload(context.Background(), "test/file.txt", data, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "/uploads/test/file.txt", url)

	content, err := os.ReadFile(filepath.Join(tmpDir, "test/file.txt"))
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestLocalStorage_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "/uploads")
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
	storage, err := NewLocalStorage(tmpDir, "/uploads")
	require.NoError(t, err)

	err = storage.Delete(context.Background(), "non-existent.txt")
	assert.NoError(t, err)
}

func TestLocalStorage_GetURL(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir, "/uploads")
	require.NoError(t, err)

	url := storage.GetURL("images/photo.jpg")
	assert.Equal(t, "/uploads/images/photo.jpg", url)
}

func TestLocalStorage_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "nested", "dir")

	storage, err := NewLocalStorage(newDir, "/uploads")
	require.NoError(t, err)
	assert.NotNil(t, storage)

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
