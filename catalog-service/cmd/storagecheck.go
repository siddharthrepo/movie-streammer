package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/logger"
	"github.com/siddharthraturi/movie-streamer/catalog-service/storage"
)

var storageCheckWrite bool

var storageCheckCmd = &cobra.Command{
	Use:   "storage-check",
	Short: "Verify object storage is reachable and the bucket is usable",
	RunE:  runStorageCheck,
}

func init() {
	storageCheckCmd.Flags().BoolVar(&storageCheckWrite, "write", false,
		"upload real bytes through a presigned URL and complete the multipart")
	rootCmd.AddCommand(storageCheckCmd)
}

func uploadPart(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("put part: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("put part: status %d: %s", resp.StatusCode, string(msg))
	}
	return resp.Header.Get("ETag"), nil
}

func runStorageCheck(cmd *cobra.Command, args []string) error {
	if err := logger.Init(global.LogLevel, global.LogFormat, "catalog-service"); err != nil {
		return err
	}
	defer logger.Sync()

	mode := "aws"
	if global.S3IsLocal {
		mode = "localstack"
	}
	logger.L().Info("storage config",
		zap.String("mode", mode),
		zap.String("endpoint", global.S3Endpoint),
		zap.String("bucket", global.S3Bucket),
		zap.String("region", global.S3Region),
		zap.Bool("path_style", global.S3UsePathStyle),
	)

	store, err := storage.NewS3()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := store.EnsureBucket(ctx); err != nil {
		return err
	}
	logger.L().Info("bucket ready")

	key := fmt.Sprintf("_healthcheck/%d.probe", time.Now().UnixNano())

	uploadID, err := store.CreateMultipartUpload(ctx, key)
	if err != nil {
		return err
	}
	logger.L().Info("multipart created", zap.String("key", key), zap.String("upload_id", uploadID))

	parts, err := store.PresignParts(ctx, key, uploadID, 2, global.UploadPresignTTL)
	if err != nil {
		return err
	}
	for _, p := range parts {
		logger.L().Info("presigned part", zap.Int32("part", p.PartNumber), zap.String("url", p.URL))
	}

	if !storageCheckWrite {
		if err := store.AbortMultipartUpload(ctx, key, uploadID); err != nil {
			return err
		}
		logger.L().Info("multipart aborted, storage seam verified")
		return nil
	}

	payload := bytes.Repeat([]byte("movie-streamer probe payload\n"), 512)
	etag, err := uploadPart(ctx, parts[0].URL, payload)
	if err != nil {
		return err
	}
	logger.L().Info("part uploaded", zap.Int("bytes", len(payload)), zap.String("etag", etag))

	if err := store.CompleteMultipartUpload(ctx, key, uploadID, []global.Part{
		{PartNumber: 1, ETag: etag},
	}); err != nil {
		return err
	}
	logger.L().Info("multipart completed")

	info, err := store.Head(ctx, key)
	if err != nil {
		return err
	}
	if info.Size != int64(len(payload)) {
		return fmt.Errorf("size mismatch: uploaded %d, stored %d", len(payload), info.Size)
	}
	logger.L().Info("object verified", zap.Int64("size", info.Size), zap.String("etag", info.ETag))

	getURL, err := store.PresignGet(ctx, key, global.UploadPresignTTL)
	if err != nil {
		return err
	}
	logger.L().Info("presigned get", zap.String("url", getURL[:80]+"..."))

	if err := store.Delete(ctx, key); err != nil {
		return err
	}
	if _, err := store.Head(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("object still present after delete: %v", err)
	}
	logger.L().Info("round trip verified: create, presign, put, complete, head, get, delete")
	return nil
}
