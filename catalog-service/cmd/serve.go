package cmd

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/siddharthraturi/movie-streamer/catalog-service/controller"
	"github.com/siddharthraturi/movie-streamer/catalog-service/database"
	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/repository"
	"github.com/siddharthraturi/movie-streamer/catalog-service/route"
	"github.com/siddharthraturi/movie-streamer/catalog-service/service"
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
	gdb, err := database.Connect()
	if err != nil {
		return err
	}

	movieRepo := repository.NewMovieRepository(gdb)
	movieSvc := service.NewMovieService(movieRepo)
	movieCtrl := controller.NewMovieController(movieSvc)

	router := route.New(movieCtrl, gdb)

	srv := &http.Server{
		Addr:              ":" + global.CatalogServicePort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("catalog-service listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("catalog-service: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Println("catalog-service: shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), global.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	sqlDB, err := gdb.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
	return nil
}
