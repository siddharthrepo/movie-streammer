package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/shared/logger"
	"github.com/siddharthraturi/movie-streamer/transcode-service/executor"
	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
	"github.com/siddharthraturi/movie-streamer/transcode-service/repository"
	"github.com/siddharthraturi/movie-streamer/transcode-service/service"
)

var (
	workConcurrency int
	workFakeMs      int
	workFailPercent int
	workMaxRuntime  time.Duration
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Claim transcode chunks and process them until interrupted",
	RunE:  runWork,
}

func init() {
	workCmd.Flags().IntVar(&workConcurrency, "concurrency", global.WorkerConcurrency, "number of worker goroutines")
	workCmd.Flags().IntVar(&workFakeMs, "fake-work-ms", int(global.FakeWorkDuration.Milliseconds()), "simulated work duration per chunk")
	workCmd.Flags().IntVar(&workFailPercent, "fail-percent", global.FakeFailPercent, "percentage of chunks to fail on purpose")
	workCmd.Flags().DurationVar(&workMaxRuntime, "max-runtime", 0, "stop after this duration (0 = run until signalled)")
	rootCmd.AddCommand(workCmd)
}

func runWork(cmd *cobra.Command, args []string) error {
	if err := logger.Init(global.LogLevel, global.LogFormat, "transcode-service"); err != nil {
		return err
	}
	defer logger.Sync()

	if workConcurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}
	if global.LeaseRenewInterval >= global.LeaseTTL {
		return fmt.Errorf("lease renew interval (%s) must be shorter than lease ttl (%s)",
			global.LeaseRenewInterval, global.LeaseTTL)
	}

	gdb, err := openDB()
	if err != nil {
		return err
	}

	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	owner := fmt.Sprintf("%s-%d", host, os.Getpid())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if workMaxRuntime > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, workMaxRuntime)
		defer stop()
	}

	svc := service.NewWorkerService(
		repository.NewChunkRepository(gdb),
		executor.NewFake(time.Duration(workFakeMs)*time.Millisecond, workFailPercent),
		owner,
	)

	logger.L().Info("worker pool starting",
		zap.String("owner", owner),
		zap.Int("concurrency", workConcurrency),
		zap.Duration("lease_ttl", global.LeaseTTL),
		zap.Duration("lease_renew", global.LeaseRenewInterval),
		zap.Int("max_attempts", global.WorkerMaxAttempts),
		zap.Int("fake_work_ms", workFakeMs),
	)

	start := time.Now()
	stats := svc.Run(ctx, workConcurrency)

	logger.L().Info("worker pool stopped",
		zap.Duration("uptime", time.Since(start)),
		zap.Int("claimed", stats.Claimed),
		zap.Int("completed", stats.Completed),
		zap.Int("retried", stats.Retried),
		zap.Int("interrupted", stats.Interrupted),
		zap.Int("failed", stats.Failed),
		zap.Int("lease_lost", stats.LeaseLost),
	)
	return nil
}
