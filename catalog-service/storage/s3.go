package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/shared/awsclient"
)

type s3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3() (Storage, error) {
	client := awsclient.NewS3(
		global.S3Region, global.S3Endpoint,
		global.S3AccessKey, global.S3SecretKey,
		global.S3UsePathStyle,
	)

	return &s3Storage{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  global.S3Bucket,
	}, nil
}

func (s *s3Storage) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	if !awsclient.IsNotFound(err) {
		return fmt.Errorf("head bucket %s: %w", s.bucket, err)
	}

	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil && !awsclient.IsAlreadyOwned(err) {
		return fmt.Errorf("create bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *s3Storage) CreateMultipartUpload(ctx context.Context, key string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart upload %s: %w", key, err)
	}
	return aws.ToString(out.UploadId), nil
}

func (s *s3Storage) PresignParts(ctx context.Context, key, uploadID string, parts int32, ttl time.Duration) ([]global.PresignedPart, error) {
	if parts < 1 || parts > global.MaxParts {
		return nil, fmt.Errorf("%w: parts must be 1..%d, got %d", ErrBadRequest, global.MaxParts, parts)
	}

	out := make([]global.PresignedPart, 0, parts)
	for n := int32(1); n <= parts; n++ {
		req, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(n),
		}, s3.WithPresignExpires(ttl))
		if err != nil {
			return nil, fmt.Errorf("presign part %d: %w", n, err)
		}
		out = append(out, global.PresignedPart{PartNumber: n, URL: req.URL})
	}
	return out, nil
}

func (s *s3Storage) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []global.Part) error {
	if len(parts) == 0 {
		return fmt.Errorf("%w: no parts supplied", ErrBadRequest)
	}

	completed := make([]types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		completed = append(completed, types.CompletedPart{
			PartNumber: aws.Int32(p.PartNumber),
			ETag:       aws.String(p.ETag),
		})
	}

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(s.bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil && !awsclient.IsNotFound(err) {
		return fmt.Errorf("abort multipart upload %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) Head(ctx context.Context, key string) (*global.ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if awsclient.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("head object %s: %w", key, err)
	}

	return &global.ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ETag:         aws.ToString(out.ETag),
		LastModified: aws.ToTime(out.LastModified),
	}, nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !awsclient.IsNotFound(err) {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", key, err)
	}
	return req.URL, nil
}
