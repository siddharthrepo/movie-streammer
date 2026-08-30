package storage

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("object not found")

type Storage interface {
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PutFile(ctx context.Context, key, contentType, path string) error
}
