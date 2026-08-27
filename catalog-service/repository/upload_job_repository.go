package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
)

var ErrStateConflict = errors.New("state conflict")

type UploadJobRepository interface {
	CreateWithMovie(ctx context.Context, m *model.Movie, j *model.UploadJob) error
	GetByID(ctx context.Context, id string) (*model.UploadJob, error)
	SetS3UploadID(ctx context.Context, id, uploadID string) error
	MarkUploaded(ctx context.Context, id string, size int64) error
	MarkFailed(ctx context.Context, id, reason string) error
	DeleteWithMovie(ctx context.Context, jobID, movieID string) error
}

type uploadJobRepository struct {
	db *gorm.DB
}

func NewUploadJobRepository(db *gorm.DB) UploadJobRepository {
	return &uploadJobRepository{db: db}
}

func (r *uploadJobRepository) CreateWithMovie(ctx context.Context, m *model.Movie, j *model.UploadJob) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("create movie: %w", err)
		}
		j.MovieID = m.ID
		if err := tx.Create(j).Error; err != nil {
			return fmt.Errorf("create upload job: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *uploadJobRepository) GetByID(ctx context.Context, id string) (*model.UploadJob, error) {
	var j model.UploadJob
	err := r.db.WithContext(ctx).First(&j, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload job: %w", err)
	}
	return &j, nil
}

func (r *uploadJobRepository) SetS3UploadID(ctx context.Context, id, uploadID string) error {
	return r.guardedUpdate(ctx, id, global.JobPendingUpload, map[string]any{
		"s3_upload_id": uploadID,
	})
}

func (r *uploadJobRepository) MarkUploaded(ctx context.Context, id string, size int64) error {
	return r.guardedUpdate(ctx, id, global.JobPendingUpload, map[string]any{
		"state":       global.JobUploaded,
		"source_size": size,
	})
}

func (r *uploadJobRepository) MarkFailed(ctx context.Context, id, reason string) error {
	res := r.db.WithContext(ctx).Model(&model.UploadJob{}).
		Where("id = ? AND state <> ?", id, global.JobCompleted).
		Updates(map[string]any{"state": global.JobFailed, "error": reason})
	if res.Error != nil {
		return fmt.Errorf("mark job failed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}

func (r *uploadJobRepository) DeleteWithMovie(ctx context.Context, jobID, movieID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.UploadJob{}, "id = ?", jobID)
		if res.Error != nil {
			return fmt.Errorf("delete upload job: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Delete(&model.Movie{}, "id = ? AND status = ?", movieID, global.StatusDraft).Error; err != nil {
			return fmt.Errorf("delete draft movie: %w", err)
		}
		return nil
	})
}

func (r *uploadJobRepository) guardedUpdate(ctx context.Context, id, fromState string, fields map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.UploadJob{}).
		Where("id = ? AND state = ?", id, fromState).
		Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("update upload job: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}
