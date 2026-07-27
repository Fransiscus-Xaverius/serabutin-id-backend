package s3

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Storage struct {
	client    *minio.Client
	bucket    string
	urlPrefix *url.URL
}

func NewS3Storage(endpoint, bucket, region, accessKey, secretKey string, useSSL bool, urlPrefix string) (*S3Storage, error) {
	publicURL, err := parsePublicURL(urlPrefix)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &S3Storage{
		client:    client,
		bucket:    bucket,
		urlPrefix: publicURL,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	reader := bytes.NewReader(data)
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload object: %w", err)
	}

	return s.GetURL(key), nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (s *S3Storage) GetURL(key string) string {
	value, err := joinPublicURL(s.urlPrefix, s.bucket, key)
	if err != nil {
		return ""
	}
	return value
}

func (s *S3Storage) ResolveImageURL(value string) string {
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

func parsePublicURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("storage S3 URL prefix must be an absolute HTTP(S) URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (scheme != "http" && scheme != "https") {
		return nil, fmt.Errorf("storage S3 URL prefix must be an absolute HTTP(S) URL")
	}
	parsed.Scheme = scheme
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed, nil
}

func joinPublicURL(base *url.URL, bucket, key string) (string, error) {
	if base == nil || bucket == "" || validateKey(key) != nil {
		return "", fmt.Errorf("invalid public object URL")
	}
	joined := *base
	joined.Path = path.Join(joined.Path, bucket, filepath.ToSlash(key))
	return joined.String(), nil
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
