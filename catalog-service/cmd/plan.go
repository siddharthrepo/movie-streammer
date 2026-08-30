package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/catalog-service/database"
	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/logger"
	"github.com/siddharthraturi/movie-streamer/catalog-service/probe"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
	"github.com/siddharthraturi/movie-streamer/catalog-service/service"
	"github.com/siddharthraturi/movie-streamer/catalog-service/storage"
)

var (
	planLimit int
	planJobID string
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Probe uploaded sources and generate transcode chunk rows",
	RunE:  runPlan,
}

func init() {
	planCmd.Flags().IntVar(&planLimit, "limit", 10, "maximum jobs to plan in this run")
	planCmd.Flags().StringVar(&planJobID, "job", "", "plan one specific job id")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	if err := logger.Init(global.LogLevel, global.LogFormat, "catalog-service"); err != nil {
		return err
	}
	defer logger.Sync()

	gdb, err := database.Connect()
	if err != nil {
		return err
	}

	store, err := storage.NewS3()
	if err != nil {
		return err
	}

	svc := service.NewPlanService(
		repository.NewUploadJobRepository(gdb),
		repository.NewMovieRepository(gdb),
		repository.NewTranscodeChunkRepository(gdb),
		store,
		probe.NewFFProbe(global.FFProbeBinary, global.ProbeTimeout),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var results []global.PlanResponse
	if planJobID != "" {
		one, err := svc.Plan(ctx, planJobID)
		if err != nil {
			return err
		}
		results = append(results, *one)
	} else {
		results, err = svc.PlanPending(ctx, planLimit)
		if err != nil {
			return err
		}
	}

	if len(results) == 0 {
		logger.L().Info("no jobs awaiting planning")
		return nil
	}

	for _, r := range results {
		logger.L().Info("planned",
			zap.String("job_id", r.JobID),
			zap.String("movie_id", r.MovieID),
			zap.Int64("duration_ms", r.DurationMs),
			zap.Int("chunks", r.ChunkCount),
			zap.Strings("qualities", r.Qualities),
			zap.Int("work_items", r.TotalItems),
		)
	}
	return nil
}
