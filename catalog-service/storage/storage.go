package storage

import (
	"context"
	"errors"
	"time"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

var (
	ErrNotFound   = errors.New("object not found")
	ErrBadRequest = errors.New("invalid storage request")
)

type Storage interface {
	EnsureBucket(ctx context.Context) error

	CreateMultipartUpload(ctx context.Context, key string) (uploadID string, err error)
	PresignParts(ctx context.Context, key, uploadID string, parts int32, ttl time.Duration) ([]global.PresignedPart, error)
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []global.Part) error
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error

	Head(ctx context.Context, key string) (*global.ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}
