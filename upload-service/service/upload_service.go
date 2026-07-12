package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/siddharthraturi/movie-streamer/upload-service/global"
	"github.com/siddharthraturi/movie-streamer/upload-service/model"
	"github.com/siddharthraturi/movie-streamer/upload-service/repository"
	"github.com/siddharthraturi/movie-streamer/upload-service/storage"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrIncomplete   = errors.New("upload incomplete")
)

type UploadService struct {
	repo       repository.MovieRepository
	store      storage.Storage
	partSize   int64
	presignTTL time.Duration
}

func NewUploadService(repo repository.MovieRepository, store storage.Storage, partSize int64, presignTTL time.Duration) *UploadService {
	return &UploadService{repo: repo, store: store, partSize: partSize, presignTTL: presignTTL}
}

func (s *UploadService) InitUpload(ctx context.Context, filename string, size int64, contentType string) (*global.InitResult, error) {
	if filename == "" || contentType == "" || size <= 0 {
		return nil, fmt.Errorf("%w: filename, content_type and a positive size are required", ErrInvalidInput)
	}
	count := partCount(size, s.partSize)
	if count > global.MaxParts {
		return nil, fmt.Errorf("%w: file needs %d parts, exceeds the %d-part limit", ErrInvalidInput, count, global.MaxParts)
	}

	id := uuid.NewString()
	key := fmt.Sprintf("movies/%s/original", id)

	uploadID, err := s.store.CreateMultipartUpload(ctx, key, contentType)
	if err != nil {
		return nil, fmt.Errorf("create multipart upload: %w", err)
	}

	m := &model.Movie{
		ID:          id,
		Filename:    filename,
		ObjectKey:   key,
		SizeBytes:   size,
		ContentType: contentType,
		Status:      model.StatusPendingUpload,
		UploadID:    uploadID,
		PartSize:    s.partSize,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		_ = s.store.AbortMultipart(ctx, key, uploadID)
		return nil, fmt.Errorf("create movie: %w", err)
	}

	return &global.InitResult{MovieID: id, UploadID: uploadID, PartSize: s.partSize, PartCount: count}, nil
}

func (s *UploadService) PresignParts(ctx context.Context, movieID string, partNumbers []int) (map[int]string, error) {
	m, err := s.getPending(ctx, movieID)
	if err != nil {
		return nil, err
	}
	count := partCount(m.SizeBytes, m.PartSize)
	urls := make(map[int]string, len(partNumbers))
	for _, n := range partNumbers {
		if n < 1 || n > count {
			return nil, fmt.Errorf("%w: part number %d out of range 1..%d", ErrInvalidInput, n, count)
		}
		u, err := s.store.PresignPart(ctx, m.ObjectKey, m.UploadID, n, s.presignTTL)
		if err != nil {
			return nil, fmt.Errorf("presign part %d: %w", n, err)
		}
		urls[n] = u
	}
	return urls, nil
}

func (s *UploadService) Status(ctx context.Context, movieID string) (*global.StatusResult, error) {
	m, err := s.get(ctx, movieID)
	if err != nil {
		return nil, err
	}
	res := &global.StatusResult{
		Status:    m.Status,
		PartSize:  m.PartSize,
		PartCount: partCount(m.SizeBytes, m.PartSize),
	}
	if m.Status == model.StatusPendingUpload {
		parts, err := s.store.ListParts(ctx, m.ObjectKey, m.UploadID)
		if err != nil {
			return nil, fmt.Errorf("list parts: %w", err)
		}
		for _, p := range parts {
			res.UploadedParts = append(res.UploadedParts, p.Number)
		}
	}
	return res, nil
}

func (s *UploadService) Complete(ctx context.Context, movieID string) error {
	m, err := s.get(ctx, movieID)
	if err != nil {
		return err
	}
	if m.Status == model.StatusUploaded {
		return nil
	}
	if m.Status != model.StatusPendingUpload {
		return fmt.Errorf("%w: cannot complete a %s upload", ErrConflict, m.Status)
	}

	parts, err := s.store.ListParts(ctx, m.ObjectKey, m.UploadID)
	if err != nil {
		return fmt.Errorf("list parts: %w", err)
	}
	count := partCount(m.SizeBytes, m.PartSize)
	if missing := missingParts(parts, count); len(missing) > 0 {
		return fmt.Errorf("%w: missing parts %v", ErrIncomplete, missing)
	}

	if err := s.store.CompleteMultipart(ctx, m.ObjectKey, m.UploadID, parts); err != nil {
		return fmt.Errorf("complete multipart: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, m.ID, model.StatusUploaded); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (s *UploadService) Abort(ctx context.Context, movieID string) error {
	m, err := s.get(ctx, movieID)
	if err != nil {
		return err
	}
	if m.Status == model.StatusPendingUpload {
		if err := s.store.AbortMultipart(ctx, m.ObjectKey, m.UploadID); err != nil {
			return fmt.Errorf("abort multipart: %w", err)
		}
	}
	if err := s.repo.UpdateStatus(ctx, m.ID, model.StatusAborted); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (s *UploadService) get(ctx context.Context, movieID string) (*model.Movie, error) {
	m, err := s.repo.GetByID(ctx, movieID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *UploadService) getPending(ctx context.Context, movieID string) (*model.Movie, error) {
	m, err := s.get(ctx, movieID)
	if err != nil {
		return nil, err
	}
	if m.Status != model.StatusPendingUpload {
		return nil, fmt.Errorf("%w: upload is %s, not pending", ErrConflict, m.Status)
	}
	return m, nil
}

func partCount(size, partSize int64) int {
	if partSize <= 0 {
		return 0
	}
	return int((size + partSize - 1) / partSize)
}

func missingParts(parts []global.Part, count int) []int {
	have := make(map[int]bool, len(parts))
	for _, p := range parts {
		have[p.Number] = true
	}
	var missing []int
	for n := 1; n <= count; n++ {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	return missing
}
