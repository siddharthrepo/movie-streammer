package storage

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/siddharthraturi/movie-streamer/upload-service/global"
)

func testStorage(t *testing.T) *MinIO {
	global.MinIOBucket = "movies-test"
	s, err := NewMinIO()
	if err != nil {
		t.Fatalf("new minio: %v", err)
	}
	if err := s.EnsureBucket(context.Background()); err != nil {
		t.Skipf("minio not reachable, skipping: %v", err)
	}
	return s
}

func TestMultipartRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	key := "test/multipart-roundtrip.bin"

	uploadID, err := s.CreateMultipartUpload(ctx, key, "application/octet-stream")
	if err != nil {
		t.Fatalf("create multipart: %v", err)
	}
	t.Cleanup(func() { _ = s.AbortMultipart(ctx, key, uploadID) })

	partURL, err := s.PresignPart(ctx, key, uploadID, 1, time.Hour)
	if err != nil {
		t.Fatalf("presign part: %v", err)
	}

	payload := bytes.Repeat([]byte("x"), 1024)
	req, _ := http.NewRequest(http.MethodPut, partURL, bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put part: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put part status: %d", resp.StatusCode)
	}

	parts, err := s.ListParts(ctx, key, uploadID)
	if err != nil {
		t.Fatalf("list parts: %v", err)
	}
	if len(parts) != 1 || parts[0].Number != 1 {
		t.Fatalf("expected 1 part #1, got %+v", parts)
	}

	if err := s.CompleteMultipart(ctx, key, uploadID, parts); err != nil {
		t.Fatalf("complete: %v", err)
	}
	t.Cleanup(func() { _ = s.core.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}) })

	info, err := s.core.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("size mismatch: got %d want %d", info.Size, len(payload))
	}
}
