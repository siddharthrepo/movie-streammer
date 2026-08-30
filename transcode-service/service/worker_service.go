package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/shared/logger"
	"github.com/siddharthraturi/movie-streamer/transcode-service/executor"
	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
	"github.com/siddharthraturi/movie-streamer/transcode-service/model"
	"github.com/siddharthraturi/movie-streamer/transcode-service/repository"
)

type WorkerService interface {
	Run(ctx context.Context, workers int) global.WorkerStats
}

type workerService struct {
	chunks repository.ChunkRepository
	exec   executor.Executor
	host   string

	mu    sync.Mutex
	stats global.WorkerStats
}

func NewWorkerService(chunks repository.ChunkRepository, exec executor.Executor, host string) WorkerService {
	return &workerService{chunks: chunks, exec: exec, host: host}
}

func (s *workerService) Run(ctx context.Context, workers int) global.WorkerStats {
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.loop(ctx, fmt.Sprintf("%s-w%d", s.host, n))
		}(i)
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *workerService) loop(ctx context.Context, owner string) {
	log := logger.L().With(zap.String("worker", owner))
	log.Info("worker started")
	defer log.Info("worker stopped")

	for ctx.Err() == nil {
		chunks, err := s.chunks.Claim(ctx, owner, global.WorkerBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("claim failed", zap.Error(err))
			if !wait(ctx, global.WorkerIdleBackoff) {
				return
			}
			continue
		}

		if len(chunks) == 0 {
			if !wait(ctx, global.WorkerIdleBackoff) {
				return
			}
			continue
		}

		s.record(func(st *global.WorkerStats) { st.Claimed += len(chunks) })
		for _, chunk := range chunks {
			s.process(ctx, owner, chunk, log)
		}

		if !wait(ctx, global.WorkerPollInterval) {
			return
		}
	}
}

func (s *workerService) process(ctx context.Context, owner string, chunk model.TranscodeChunk, log *zap.Logger) {
	log = log.With(
		zap.Int64("chunk", chunk.ID),
		zap.String("job_id", chunk.JobID),
		zap.Int("index", chunk.ChunkIndex),
		zap.String("quality", chunk.Quality),
		zap.Int("attempt", chunk.Attempts),
	)
	log.Info("claimed")

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	lost := false
	renewed := make(chan struct{})

	go func() {
		defer close(renewed)
		ticker := time.NewTicker(global.LeaseRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(context.WithoutCancel(workCtx), global.LeaseTTL)
				err := s.chunks.RenewLease(renewCtx, chunk.ID, owner)
				stop()
				if err != nil {
					lost = true
					log.Warn("lease renewal failed, abandoning chunk", zap.Error(err))
					cancel()
					return
				}
				log.Debug("lease renewed")
			}
		}
	}()

	start := time.Now()
	execErr := s.exec.Execute(workCtx, chunk)
	cancel()
	<-renewed

	if lost {
		s.record(func(st *global.WorkerStats) { st.LeaseLost++ })
		return
	}

	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), global.ShutdownDrainTimeout)
	defer stop()

	if execErr == nil {
		if err := s.chunks.MarkDone(finish, chunk.ID, owner); err != nil {
			s.recordLeaseResult(err, log, "mark done")
			return
		}
		s.record(func(st *global.WorkerStats) { st.Completed++ })
		log.Info("done", zap.Duration("took", time.Since(start)))
		return
	}

	if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) {
		if err := s.chunks.Requeue(finish, chunk.ID, owner); err != nil {
			s.recordLeaseResult(err, log, "requeue")
			return
		}
		s.record(func(st *global.WorkerStats) { st.Interrupted++ })
		log.Info("interrupted by shutdown, requeued without consuming an attempt")
		return
	}

	terminal := chunk.Attempts >= global.WorkerMaxAttempts
	if err := s.chunks.Release(finish, chunk.ID, owner, execErr.Error(), terminal); err != nil {
		s.recordLeaseResult(err, log, "release")
		return
	}

	if terminal {
		s.record(func(st *global.WorkerStats) { st.Failed++ })
		log.Error("chunk failed permanently", zap.Error(execErr))
		return
	}
	s.record(func(st *global.WorkerStats) { st.Retried++ })
	log.Warn("chunk returned to pending", zap.Error(execErr))
}

func (s *workerService) recordLeaseResult(err error, log *zap.Logger, action string) {
	if errors.Is(err, repository.ErrLeaseLost) {
		s.record(func(st *global.WorkerStats) { st.LeaseLost++ })
		log.Warn("lease was taken by another worker", zap.String("action", action))
		return
	}
	log.Error("chunk state update failed", zap.String("action", action), zap.Error(err))
}

func (s *workerService) record(fn func(*global.WorkerStats)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.stats)
}

func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
