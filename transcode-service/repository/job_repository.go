package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrJobNotFound = errors.New("upload job not found")

type JobRepository interface {
	SourceKey(ctx context.Context, jobID string) (string, error)
}

type jobRepository struct {
	db *gorm.DB
}

func NewJobRepository(db *gorm.DB) JobRepository {
	return &jobRepository{db: db}
}

func (r *jobRepository) SourceKey(ctx context.Context, jobID string) (string, error) {
	var key string
	err := r.db.WithContext(ctx).
		Raw("SELECT source_key FROM upload_jobs WHERE id = ?", jobID).
		Scan(&key).Error
	if err != nil {
		return "", fmt.Errorf("read source key: %w", err)
	}
	if key == "" {
		return "", ErrJobNotFound
	}
	return key, nil
}
