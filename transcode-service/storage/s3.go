package storage

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/siddharthraturi/movie-streamer/shared/awsclient"
	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
)

type s3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3() Storage {
	client := awsclient.NewS3(
		global.S3Region, global.S3Endpoint,
		global.S3AccessKey, global.S3SecretKey,
		global.S3UsePathStyle,
	)
	return &s3Storage{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  global.S3Bucket,
	}
}

func (s *s3Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	out, err := s.presign.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl),
	)
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", key, err)
	}
	return out.URL, nil
}

func (s *s3Storage) PutFile(ctx context.Context, key, contentType, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(info.Size()),
	})
	if err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}
