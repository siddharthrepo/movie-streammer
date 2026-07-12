package storage

import (
	"context"
	"time"
)

type Part struct {
	Number int
	ETag   string
	Size   int64
}

type Storage interface {
	EnsureBucket(ctx context.Context) error
	CreateMultipartUpload(ctx context.Context, key, contentType string) (uploadID string, err error)
	PresignPart(ctx context.Context, key, uploadID string, partNumber int, ttl time.Duration) (string, error)
	ListParts(ctx context.Context, key, uploadID string) ([]Part, error)
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []Part) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
}
