package repository

import "context"

type StorageRepository interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	GetURL(key string) string
}
