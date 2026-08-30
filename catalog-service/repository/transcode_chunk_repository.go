package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/model"
)

type TranscodeChunkRepository interface {
	BulkInsert(ctx context.Context, chunks []model.TranscodeChunk) error
	CountByJob(ctx context.Context, jobID string) (global.ChunkCounts, error)
	DeleteByJob(ctx context.Context, jobID string) error
}

type transcodeChunkRepository struct {
	db *gorm.DB
}

func NewTranscodeChunkRepository(db *gorm.DB) TranscodeChunkRepository {
	return &transcodeChunkRepository{db: db}
}

func (r *transcodeChunkRepository) BulkInsert(ctx context.Context, chunks []model.TranscodeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(chunks, 500).Error
	if err != nil {
		return fmt.Errorf("bulk insert chunks: %w", err)
	}
	return nil
}

func (r *transcodeChunkRepository) CountByJob(ctx context.Context, jobID string) (global.ChunkCounts, error) {
	var rows []struct {
		State string
		N     int
	}
	err := r.db.WithContext(ctx).Model(&model.TranscodeChunk{}).
		Select("state, COUNT(*) AS n").
		Where("job_id = ?", jobID).
		Group("state").Scan(&rows).Error
	if err != nil {
		return global.ChunkCounts{}, fmt.Errorf("count chunks: %w", err)
	}

	var c global.ChunkCounts
	for _, row := range rows {
		c.Total += row.N
		switch row.State {
		case global.ChunkPending:
			c.Pending = row.N
		case global.ChunkLeased:
			c.Leased = row.N
		case global.ChunkDone:
			c.Done = row.N
		case global.ChunkFailed:
			c.Failed = row.N
		}
	}
	return c, nil
}

func (r *transcodeChunkRepository) DeleteByJob(ctx context.Context, jobID string) error {
	err := r.db.WithContext(ctx).Delete(&model.TranscodeChunk{}, "job_id = ?", jobID).Error
	if err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}
