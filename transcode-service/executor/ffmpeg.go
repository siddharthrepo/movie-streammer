package executor

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
	"github.com/siddharthraturi/movie-streamer/transcode-service/model"
	"github.com/siddharthraturi/movie-streamer/transcode-service/storage"
)

type SourceLocator interface {
	SourceKey(ctx context.Context, jobID string) (string, error)
}

type ffmpegExecutor struct {
	store   storage.Storage
	sources SourceLocator
	binary  string
	workDir string
}

func NewFFmpeg(store storage.Storage, sources SourceLocator, binary, workDir string) Executor {
	return &ffmpegExecutor{store: store, sources: sources, binary: binary, workDir: workDir}
}

func (e *ffmpegExecutor) Execute(ctx context.Context, chunk model.TranscodeChunk) error {
	rendition, err := renditionFor(chunk.Quality)
	if err != nil {
		return err
	}

	sourceKey, err := e.sources.SourceKey(ctx, chunk.JobID)
	if err != nil {
		return err
	}

	sourceURL, err := e.store.PresignGet(ctx, sourceKey, global.SourcePresignTTL)
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp(e.workDir, fmt.Sprintf("chunk-%d-", chunk.ID))
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(dir)

	run, cancel := context.WithTimeout(ctx, global.TranscodeTimeout)
	defer cancel()

	cmd := exec.CommandContext(run, e.binary, ffmpegArgs(chunk, rendition, sourceURL, dir)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(redact(stderr.String(), sourceURL))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("ffmpeg chunk %d %s: %s", chunk.ChunkIndex, chunk.Quality, detail)
	}

	segments, err := filepath.Glob(filepath.Join(dir, "seg_*.ts"))
	if err != nil {
		return fmt.Errorf("list segments: %w", err)
	}
	sort.Strings(segments)

	if err := verifyCount(chunk, len(segments)); err != nil {
		return err
	}

	for _, path := range segments {
		key := fmt.Sprintf("movies/%s/hls/%s/%s", chunk.MovieID, chunk.Quality, filepath.Base(path))
		if err := e.store.PutFile(ctx, key, "video/mp2t", path); err != nil {
			return err
		}
	}
	return nil
}

func ffmpegArgs(chunk model.TranscodeChunk, r global.Rendition, sourceURL, dir string) []string {
	startSec := float64(chunk.StartMs) / 1000
	durationSec := float64(chunk.EndMs-chunk.StartMs) / 1000

	return []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-i", sourceURL,
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-vf", fmt.Sprintf("scale=-2:%d", r.Height),
		"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main",
		"-b:v", fmt.Sprintf("%d", r.VideoBitrate),
		"-maxrate", fmt.Sprintf("%d", r.VideoBitrate*107/100),
		"-bufsize", fmt.Sprintf("%d", r.VideoBitrate*2),
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", global.KeyframeSeconds),
		"-sc_threshold", "0",
		"-c:a", "aac", "-b:a", fmt.Sprintf("%d", r.AudioBitrate), "-ac", "2",
		"-output_ts_offset", fmt.Sprintf("%.3f", startSec),
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", global.SegmentSeconds),
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "mpegts",
		"-hls_flags", "independent_segments",
		"-start_number", fmt.Sprintf("%d", chunk.SegmentOffset),
		"-hls_segment_filename", filepath.Join(dir, "seg_%05d.ts"),
		filepath.Join(dir, "chunk.m3u8"),
	}
}

func verifyCount(chunk model.TranscodeChunk, got int) error {
	durationSec := int((chunk.EndMs - chunk.StartMs) / 1000)
	want := (durationSec + global.SegmentSeconds - 1) / global.SegmentSeconds

	if durationSec%global.SegmentSeconds == 0 {
		if got != want {
			return fmt.Errorf("chunk %d %s: expected %d segments, got %d",
				chunk.ChunkIndex, chunk.Quality, want, got)
		}
		return nil
	}

	if got != want && got != want-1 {
		return fmt.Errorf("chunk %d %s: expected %d or %d segments, got %d",
			chunk.ChunkIndex, chunk.Quality, want-1, want, got)
	}
	return nil
}

func renditionFor(quality string) (global.Rendition, error) {
	for _, r := range global.Ladder {
		if r.Name == quality {
			return r, nil
		}
	}
	return global.Rendition{}, fmt.Errorf("unknown quality %q", quality)
}

func redact(text, raw string) string {
	cleaned := strings.ReplaceAll(text, raw, safeURL(raw))
	if i := strings.Index(cleaned, "X-Amz-Signature="); i >= 0 {
		cleaned = cleaned[:i] + "X-Amz-Signature=REDACTED"
	}
	return cleaned
}

func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<source>"
	}
	u.RawQuery = ""
	u.User = nil
	return u.Redacted()
}
