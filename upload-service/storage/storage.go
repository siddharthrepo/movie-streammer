package storage

import (
	"context"
	"time"

	"github.com/siddharthraturi/movie-streamer/upload-service/global"
)

type Storage interface {
	EnsureBucket(ctx context.Context) error
	CreateMultipartUpload(ctx context.Context, key, contentType string) (uploadID string, err error)
	PresignPart(ctx context.Context, key, uploadID string, partNumber int, ttl time.Duration) (string, error)
	ListParts(ctx context.Context, key, uploadID string) ([]global.Part, error)
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []global.Part) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
}
