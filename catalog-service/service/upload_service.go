package service

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
	"github.com/siddharthraturi/movie-streamer/catalog-service/storage"
)

var (
	ErrConflict = errors.New("upload job is not in a state that allows this")
	ErrStorage  = errors.New("object storage error")
	ErrMismatch = errors.New("uploaded object does not match the declared size")
)

type UploadService interface {
	Initiate(ctx context.Context, req global.InitiateUploadRequest) (*global.InitiateUploadResponse, error)
	Complete(ctx context.Context, id string, parts []global.Part) (*model.UploadJob, error)
	Get(ctx context.Context, id string) (*model.UploadJob, error)
	Abort(ctx context.Context, id string) error
}

type uploadService struct {
	jobs  repository.UploadJobRepository
	store storage.Storage
}

func NewUploadService(jobs repository.UploadJobRepository, store storage.Storage) UploadService {
	return &uploadService{jobs: jobs, store: store}
}

func planParts(fileSize int64) (partSize int64, partCount int, err error) {
	if fileSize < 1 || fileSize > global.MaxFileSize {
		return 0, 0, fmt.Errorf("%w: file_size must be 1..%d", ErrInvalid, int64(global.MaxFileSize))
	}

	partSize = global.UploadPartSize
	if partSize < global.MinPartSize {
		partSize = global.MinPartSize
	}

	needed := (fileSize + partSize - 1) / partSize
	if needed > global.MaxParts {
		partSize = (fileSize + global.MaxParts - 1) / global.MaxParts
		if partSize < global.MinPartSize {
			partSize = global.MinPartSize
		}
		needed = (fileSize + partSize - 1) / partSize
	}
	if needed > global.MaxParts {
		return 0, 0, fmt.Errorf("%w: file too large to split into %d parts", ErrInvalid, global.MaxParts)
	}

	return partSize, int(needed), nil
}

func sourceKey(movieID, fileName string) string {
	ext := strings.ToLower(path.Ext(fileName))
	if ext == "" || len(ext) > 8 {
		ext = ".bin"
	}
	return fmt.Sprintf("movies/%s/source/original%s", movieID, ext)
}

func (s *uploadService) Initiate(ctx context.Context, req global.InitiateUploadRequest) (*global.InitiateUploadResponse, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, ErrInvalid
	}

	partSize, partCount, err := planParts(req.FileSize)
	if err != nil {
		return nil, err
	}

	movie := &model.Movie{
		ID:          uuid.NewString(),
		Title:       title,
		Description: strings.TrimSpace(req.Description),
		Status:      global.StatusDraft,
	}
	job := &model.UploadJob{
		State:     global.JobPendingUpload,
		SourceKey: sourceKey(movie.ID, req.FileName),
		PartSize:  partSize,
		PartCount: partCount,
	}

	if err := s.jobs.CreateWithMovie(ctx, movie, job); err != nil {
		return nil, err
	}

	uploadID, err := s.store.CreateMultipartUpload(ctx, job.SourceKey)
	if err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, err.Error())
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	if err := s.jobs.SetS3UploadID(ctx, job.ID, uploadID); err != nil {
		_ = s.store.AbortMultipartUpload(ctx, job.SourceKey, uploadID)
		return nil, err
	}

	parts, err := s.store.PresignParts(ctx, job.SourceKey, uploadID, int32(partCount), global.UploadPresignTTL)
	if err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, err.Error())
		_ = s.store.AbortMultipartUpload(ctx, job.SourceKey, uploadID)
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	return &global.InitiateUploadResponse{
		UploadID:  job.ID,
		MovieID:   movie.ID,
		SourceKey: job.SourceKey,
		PartSize:  partSize,
		PartCount: partCount,
		ExpiresAt: time.Now().UTC().Add(global.UploadPresignTTL).Format(time.RFC3339),
		Parts:     parts,
	}, nil
}

func (s *uploadService) Complete(ctx context.Context, id string, parts []global.Part) (*model.UploadJob, error) {
	job, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.State != global.JobPendingUpload {
		return nil, ErrConflict
	}
	if job.S3UploadID == nil {
		return nil, ErrConflict
	}
	if len(parts) != job.PartCount {
		return nil, fmt.Errorf("%w: expected %d parts, got %d", ErrInvalid, job.PartCount, len(parts))
	}

	if err := s.store.CompleteMultipartUpload(ctx, job.SourceKey, *job.S3UploadID, parts); err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, err.Error())
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	info, err := s.store.Head(ctx, job.SourceKey)
	if err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, err.Error())
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	if err := s.jobs.MarkUploaded(ctx, job.ID, info.Size); err != nil {
		if errors.Is(err, repository.ErrStateConflict) {
			return nil, ErrConflict
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

func (s *uploadService) Get(ctx context.Context, id string) (*model.UploadJob, error) {
	job, err := s.jobs.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	return job, err
}

func (s *uploadService) Abort(ctx context.Context, id string) error {
	job, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if job.State == global.JobProcessing || job.State == global.JobCompleted {
		return ErrConflict
	}

	if job.S3UploadID != nil {
		if err := s.store.AbortMultipartUpload(ctx, job.SourceKey, *job.S3UploadID); err != nil {
			return fmt.Errorf("%w: %v", ErrStorage, err)
		}
	}
	return s.jobs.DeleteWithMovie(ctx, job.ID, job.MovieID)
}
