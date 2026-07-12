package storage

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/siddharthraturi/movie-streamer/upload-service/global"
)

var _ Storage = (*MinIO)(nil)

type MinIO struct {
	core   *minio.Core
	bucket string
}

func NewMinIO() (*MinIO, error) {
	core, err := minio.NewCore(global.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(global.MinIOAccessKey, global.MinIOSecretKey, ""),
		Secure: global.MinIOUseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &MinIO{core: core, bucket: global.MinIOBucket}, nil
}

func (s *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := s.core.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s.core.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

func (s *MinIO) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	return s.core.NewMultipartUpload(ctx, s.bucket, key, minio.PutObjectOptions{ContentType: contentType})
}

func (s *MinIO) PresignPart(ctx context.Context, key, uploadID string, partNumber int, ttl time.Duration) (string, error) {
	params := url.Values{}
	params.Set("uploadId", uploadID)
	params.Set("partNumber", strconv.Itoa(partNumber))

	u, err := s.core.Presign(ctx, http.MethodPut, s.bucket, key, ttl, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *MinIO) ListParts(ctx context.Context, key, uploadID string) ([]global.Part, error) {
	var parts []global.Part
	marker := 0
	for {
		res, err := s.core.ListObjectParts(ctx, s.bucket, key, uploadID, marker, 1000)
		if err != nil {
			return nil, err
		}
		for _, p := range res.ObjectParts {
			parts = append(parts, global.Part{Number: p.PartNumber, ETag: p.ETag, Size: p.Size})
		}
		if !res.IsTruncated {
			break
		}
		marker = res.NextPartNumberMarker
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	return parts, nil
}

func (s *MinIO) CompleteMultipart(ctx context.Context, key, uploadID string, parts []global.Part) error {
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	cp := make([]minio.CompletePart, len(parts))
	for i, p := range parts {
		cp[i] = minio.CompletePart{PartNumber: p.Number, ETag: p.ETag}
	}
	_, err := s.core.CompleteMultipartUpload(ctx, s.bucket, key, uploadID, cp, minio.PutObjectOptions{})
	return err
}

func (s *MinIO) AbortMultipart(ctx context.Context, key, uploadID string) error {
	return s.core.AbortMultipartUpload(ctx, s.bucket, key, uploadID)
}
