package cmd

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/catalog-service/controller"
	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
	"github.com/siddharthraturi/movie-streamer/catalog-service/route"
	"github.com/siddharthraturi/movie-streamer/catalog-service/service"
	"github.com/siddharthraturi/movie-streamer/catalog-service/storage"
	"github.com/siddharthraturi/movie-streamer/shared/logger"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	if err := logger.Init(global.LogLevel, global.LogFormat, "catalog-service"); err != nil {
		return err
	}
	defer logger.Sync()

	gdb, err := openDB()
	if err != nil {
		return err
	}

	store, err := storage.NewS3()
	if err != nil {
		return err
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		return err
	}
	logger.L().Info("object storage ready",
		zap.String("bucket", global.S3Bucket),
		zap.Bool("localstack", global.S3IsLocal),
	)

	movieRepo := repository.NewMovieRepository(gdb)
	movieSvc := service.NewMovieService(movieRepo)
	movieCtrl := controller.NewMovieController(movieSvc)

	jobRepo := repository.NewUploadJobRepository(gdb)
	uploadSvc := service.NewUploadService(jobRepo, store)
	uploadCtrl := controller.NewUploadController(uploadSvc)

	router := route.New(movieCtrl, uploadCtrl, gdb)

	srv := &http.Server{
		Addr:              ":" + global.CatalogServicePort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.L().Info("listening", zap.String("addr", srv.Addr), zap.String("mode", global.GinMode))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.L().Error("server stopped", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.L().Info("shutting down", zap.Duration("timeout", global.ShutdownTimeout))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), global.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	sqlDB, err := gdb.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
	logger.L().Info("stopped")
	return nil
}
