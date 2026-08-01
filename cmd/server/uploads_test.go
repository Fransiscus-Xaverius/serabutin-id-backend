package main

import (
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/config"
	"github.com/serabutin/serabutin-id-backend/internal/repository/local"
)

func TestRegisterLocalUploads(t *testing.T) {
	parent, delegated := staticTestDir(t)
	if delegated {
		return
	}
	root := filepath.Join(parent, "uploads")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "images"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "images", "photo.jpg"), []byte("image-bytes"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret"), 0644))

	app := fiber.New()
	t.Cleanup(func() { _ = app.Shutdown() })
	registerLocalUploads(app, &config.Config{StorageDriver: "local", StorageLocalPath: root, StorageLocalURL: "http://media.example.test/uploads"})

	for _, method := range []string{fiber.MethodGet, fiber.MethodHead} {
		resp, err := app.Test(httptest.NewRequest(method, "/uploads/images/photo.jpg", nil))
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		assert.Equal(t, "cross-origin", resp.Header.Get("Cross-Origin-Resource-Policy"))
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}

	for _, path := range []string{"/uploads/missing.jpg", "/uploads/images/", "/uploads/%2e%2e/secret.txt", "/uploads/..%2fsecret.txt"} {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusNotFound, resp.StatusCode, path)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
}

func TestLocalStorageUpload_IsServedAtReturnedURL(t *testing.T) {
	root, delegated := staticTestDir(t)
	if delegated {
		return
	}

	for _, tt := range []struct {
		name    string
		baseURL string
	}{
		{"duplicate separators", "http://media.example.test/media//uploads"},
		{"dot segments", "http://media.example.test/public/../media/uploads"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caseRoot := filepath.Join(root, strings.ReplaceAll(tt.name, " ", "-"))
			storage, err := local.NewLocalStorage(caseRoot, tt.baseURL)
			require.NoError(t, err)

			returnedURL, err := storage.Upload(t.Context(), "images/photo.jpg", []byte("image-bytes"), "image/jpeg")
			require.NoError(t, err)
			parsedURL, err := url.Parse(returnedURL)
			require.NoError(t, err)
			assert.Equal(t, "/media/uploads/images/photo.jpg", parsedURL.Path)

			app := fiber.New()
			t.Cleanup(func() { _ = app.Shutdown() })
			registerLocalUploads(app, &config.Config{StorageDriver: "local", StorageLocalPath: caseRoot, StorageLocalURL: tt.baseURL})

			resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, parsedURL.RequestURI(), nil))
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusOK, resp.StatusCode)
			assert.Equal(t, "image/jpeg", resp.Header.Get(fiber.HeaderContentType))
			assert.Equal(t, []byte("image-bytes"), body)
		})
	}
}

func staticTestDir(t *testing.T) (string, bool) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return t.TempDir(), false
	}
	if root := os.Getenv("SERABUTIN_STATIC_TEST_ROOT"); root != "" {
		return root, false
	}

	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$")
	cmd.Env = append(os.Environ(), "SERABUTIN_STATIC_TEST_ROOT="+root)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return "", true
}

func TestRegisterLocalUploads_DisabledForS3(t *testing.T) {
	app := fiber.New()
	t.Cleanup(func() { _ = app.Shutdown() })
	registerLocalUploads(app, &config.Config{StorageDriver: "s3", StorageLocalPath: t.TempDir(), StorageLocalURL: "http://media.example.test/uploads"})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/uploads/image.jpg", nil))
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}
