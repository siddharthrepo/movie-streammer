package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
	"github.com/siddharthraturi/movie-streamer/transcode-service/model"
)

var ErrLeaseLost = errors.New("lease no longer held")

type ChunkRepository interface {
	Claim(ctx context.Context, owner string, limit int) ([]model.TranscodeChunk, error)
	RenewLease(ctx context.Context, id int64, owner string) error
	MarkDone(ctx context.Context, id int64, owner string) error
	Release(ctx context.Context, id int64, owner, cause string, terminal bool) error
	Requeue(ctx context.Context, id int64, owner string) error
	Progress(ctx context.Context, jobID string) (global.JobProgress, error)
}

type chunkRepository struct {
	db *gorm.DB
}

func NewChunkRepository(db *gorm.DB) ChunkRepository {
	return &chunkRepository{db: db}
}

func (r *chunkRepository) Claim(ctx context.Context, owner string, limit int) ([]model.TranscodeChunk, error) {
	var claimed []model.TranscodeChunk

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []int64
		err := tx.Raw(`
			SELECT id FROM transcode_chunks
			WHERE attempts < ?
			  AND (state = ?
			       OR (state = ? AND lease_expires_at < DATE_SUB(NOW(3), INTERVAL ? SECOND)))
			ORDER BY job_id, chunk_index, quality
			LIMIT ?
			FOR UPDATE SKIP LOCKED`,
			global.WorkerMaxAttempts,
			global.ChunkPending,
			global.ChunkLeased,
			int(global.LeaseReclaimGrace.Seconds()),
			limit,
		).Scan(&ids).Error
		if err != nil {
			return fmt.Errorf("select claimable chunks: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		err = tx.Model(&model.TranscodeChunk{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"state":            global.ChunkLeased,
				"lease_owner":      owner,
				"lease_expires_at": gorm.Expr("DATE_ADD(NOW(3), INTERVAL ? SECOND)", int(global.LeaseTTL.Seconds())),
				"attempts":         gorm.Expr("attempts + 1"),
				"updated_at":       gorm.Expr("NOW(3)"),
			}).Error
		if err != nil {
			return fmt.Errorf("lease chunks: %w", err)
		}

		if err := tx.Where("id IN ?", ids).Find(&claimed).Error; err != nil {
			return fmt.Errorf("load leased chunks: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *chunkRepository) RenewLease(ctx context.Context, id int64, owner string) error {
	return r.guarded(ctx, id, owner, map[string]any{
		"lease_expires_at": gorm.Expr("DATE_ADD(NOW(3), INTERVAL ? SECOND)", int(global.LeaseTTL.Seconds())),
		"updated_at":       gorm.Expr("NOW(3)"),
	})
}

func (r *chunkRepository) MarkDone(ctx context.Context, id int64, owner string) error {
	return r.guarded(ctx, id, owner, map[string]any{
		"state":            global.ChunkDone,
		"lease_owner":      nil,
		"lease_expires_at": nil,
		"error":            nil,
		"updated_at":       gorm.Expr("NOW(3)"),
	})
}

func (r *chunkRepository) Release(ctx context.Context, id int64, owner, cause string, terminal bool) error {
	state := global.ChunkPending
	if terminal {
		state = global.ChunkFailed
	}
	if len(cause) > 1000 {
		cause = cause[:1000]
	}
	return r.guarded(ctx, id, owner, map[string]any{
		"state":            state,
		"lease_owner":      nil,
		"lease_expires_at": nil,
		"error":            cause,
		"updated_at":       gorm.Expr("NOW(3)"),
	})
}

func (r *chunkRepository) Requeue(ctx context.Context, id int64, owner string) error {
	return r.guarded(ctx, id, owner, map[string]any{
		"state":            global.ChunkPending,
		"lease_owner":      nil,
		"lease_expires_at": nil,
		"attempts":         gorm.Expr("GREATEST(attempts - 1, 0)"),
		"updated_at":       gorm.Expr("NOW(3)"),
	})
}

func (r *chunkRepository) guarded(ctx context.Context, id int64, owner string, fields map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.TranscodeChunk{}).
		Where("id = ? AND state = ? AND lease_owner = ?", id, global.ChunkLeased, owner).
		Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("update chunk %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (r *chunkRepository) Progress(ctx context.Context, jobID string) (global.JobProgress, error) {
	var rows []struct {
		State string
		N     int
	}
	err := r.db.WithContext(ctx).Model(&model.TranscodeChunk{}).
		Select("state, COUNT(*) AS n").
		Where("job_id = ?", jobID).
		Group("state").Scan(&rows).Error
	if err != nil {
		return global.JobProgress{}, fmt.Errorf("chunk progress: %w", err)
	}

	p := global.JobProgress{JobID: jobID}
	for _, row := range rows {
		p.Total += row.N
		switch row.State {
		case global.ChunkPending:
			p.Pending = row.N
		case global.ChunkLeased:
			p.Leased = row.N
		case global.ChunkDone:
			p.Done = row.N
		case global.ChunkFailed:
			p.Failed = row.N
		}
	}
	return p, nil
}
