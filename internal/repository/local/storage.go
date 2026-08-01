package local

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	basePath string
	baseURL  *url.URL
}

func NewLocalStorage(basePath, baseURL string) (*LocalStorage, error) {
	publicURL, err := ParsePublicURL(baseURL)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &LocalStorage{
		basePath: basePath,
		baseURL:  publicURL,
	}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	fullPath := filepath.Join(s.basePath, key)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return s.GetURL(key), nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	fullPath := filepath.Join(s.basePath, key)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

func (s *LocalStorage) GetURL(key string) string {
	if err := validateKey(key); err != nil {
		return ""
	}
	joined := *s.baseURL
	joined.Path = path.Join(joined.Path, filepath.ToSlash(key))
	return joined.String()
}

func (s *LocalStorage) ResolveImageURL(value string) string {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") {
		return value
	}
	if strings.HasPrefix(value, "/uploads/") {
		return s.GetURL(strings.TrimPrefix(value, "/uploads/"))
	}
	if strings.HasPrefix(value, "uploads/") {
		return s.GetURL(strings.TrimPrefix(value, "uploads/"))
	}
	return value
}

func ParsePublicURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("storage local URL must be an absolute HTTP(S) URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || (scheme != "http" && scheme != "https") {
		return nil, fmt.Errorf("storage local URL must be an absolute HTTP(S) URL")
	}
	parsed.Scheme = scheme
	parsed.Path = path.Clean(parsed.Path)
	if parsed.Path == "." || parsed.Path == "/" {
		return nil, fmt.Errorf("storage local URL must be an absolute HTTP(S) URL")
	}
	return parsed, nil
}

func validateKey(key string) error {
	unescaped, err := url.PathUnescape(key)
	if err != nil || key == "" || strings.ContainsAny(unescaped, `\\`) || filepath.IsAbs(unescaped) || filepath.VolumeName(unescaped) != "" {
		return fmt.Errorf("invalid storage key")
	}
	parsed, err := url.Parse(unescaped)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return fmt.Errorf("invalid storage key")
	}
	for _, part := range strings.Split(filepath.ToSlash(unescaped), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid storage key")
		}
	}
	return nil
}
