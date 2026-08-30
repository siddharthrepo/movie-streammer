package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
	"github.com/siddharthraturi/movie-streamer/catalog-service/probe"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
	"github.com/siddharthraturi/movie-streamer/catalog-service/storage"
)

var ErrUnplayable = errors.New("source cannot be transcoded")

type PlanService interface {
	Plan(ctx context.Context, jobID string) (*global.PlanResponse, error)
	PlanPending(ctx context.Context, limit int) ([]global.PlanResponse, error)
}

type planService struct {
	jobs   repository.UploadJobRepository
	movies repository.MovieRepository
	chunks repository.TranscodeChunkRepository
	store  storage.Storage
	prober probe.Prober
}

func NewPlanService(
	jobs repository.UploadJobRepository,
	movies repository.MovieRepository,
	chunks repository.TranscodeChunkRepository,
	store storage.Storage,
	prober probe.Prober,
) PlanService {
	return &planService{jobs: jobs, movies: movies, chunks: chunks, store: store, prober: prober}
}

func renditionsFor(sourceHeight int) []global.Rendition {
	out := make([]global.Rendition, 0, len(global.Ladder))
	for _, r := range global.Ladder {
		if r.Height <= sourceHeight {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		out = append(out, global.Ladder[0])
	}
	return out
}

func buildChunks(job *model.UploadJob, info *global.MediaInfo) ([]model.TranscodeChunk, []string, int) {
	chunkMs := int64(global.ChunkSeconds) * 1000
	chunkCount := int((info.DurationMs + chunkMs - 1) / chunkMs)
	segmentsPerChunk := global.ChunkSeconds / global.SegmentSeconds

	renditions := renditionsFor(info.Height)
	names := make([]string, 0, len(renditions))
	for _, r := range renditions {
		names = append(names, r.Name)
	}

	rows := make([]model.TranscodeChunk, 0, chunkCount*len(renditions))
	for i := 0; i < chunkCount; i++ {
		start := int64(i) * chunkMs
		end := start + chunkMs
		if end > info.DurationMs {
			end = info.DurationMs
		}
		for _, r := range renditions {
			rows = append(rows, model.TranscodeChunk{
				JobID:         job.ID,
				MovieID:       job.MovieID,
				ChunkIndex:    i,
				Quality:       r.Name,
				StartMs:       start,
				EndMs:         end,
				SegmentOffset: i * segmentsPerChunk,
				State:         global.ChunkPending,
			})
		}
	}
	return rows, names, chunkCount
}

func (s *planService) Plan(ctx context.Context, jobID string) (*global.PlanResponse, error) {
	if global.ChunkSeconds%global.SegmentSeconds != 0 || global.SegmentSeconds%global.KeyframeSeconds != 0 {
		return nil, fmt.Errorf("%w: chunk/segment/keyframe durations must divide evenly (%d/%d/%d)",
			ErrInvalid, global.ChunkSeconds, global.SegmentSeconds, global.KeyframeSeconds)
	}

	job, err := s.jobs.GetByID(ctx, jobID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if job.State != global.JobUploaded {
		return nil, ErrConflict
	}

	url, err := s.store.PresignGet(ctx, job.SourceKey, global.UploadPresignTTL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	info, err := s.prober.Inspect(ctx, url)
	if err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, err.Error())
		return nil, fmt.Errorf("%w: %v", ErrUnplayable, err)
	}

	if err := s.movies.SetDuration(ctx, job.MovieID, info.DurationMs); err != nil {
		return nil, err
	}

	rows, names, chunkCount := buildChunks(job, info)
	if err := s.chunks.BulkInsert(ctx, rows); err != nil {
		return nil, err
	}

	if err := s.jobs.MarkProcessing(ctx, job.ID, chunkCount); err != nil {
		if errors.Is(err, repository.ErrStateConflict) {
			return nil, ErrConflict
		}
		return nil, err
	}

	return &global.PlanResponse{
		JobID:      job.ID,
		MovieID:    job.MovieID,
		DurationMs: info.DurationMs,
		ChunkCount: chunkCount,
		Qualities:  names,
		TotalItems: len(rows),
	}, nil
}

func (s *planService) PlanPending(ctx context.Context, limit int) ([]global.PlanResponse, error) {
	jobs, err := s.jobs.ListByState(ctx, global.JobUploaded, limit)
	if err != nil {
		return nil, err
	}

	out := make([]global.PlanResponse, 0, len(jobs))
	for i := range jobs {
		resp, err := s.Plan(ctx, jobs[i].ID)
		if err != nil {
			return out, err
		}
		out = append(out, *resp)
	}
	return out, nil
}
