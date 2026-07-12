package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/upload-service/config"
	"github.com/siddharthraturi/movie-streamer/upload-service/controller"
	"github.com/siddharthraturi/movie-streamer/upload-service/database"
	"github.com/siddharthraturi/movie-streamer/upload-service/repository"
	"github.com/siddharthraturi/movie-streamer/upload-service/service"
	"github.com/siddharthraturi/movie-streamer/upload-service/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("upload-service: config: %v", err)
	}

	gdb, err := database.Connect(cfg.Postgres)
	if err != nil {
		log.Fatalf("upload-service: database: %v", err)
	}

	store, err := storage.NewMinIO(cfg.MinIO)
	if err != nil {
		log.Fatalf("upload-service: storage: %v", err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		log.Fatalf("upload-service: ensure bucket: %v", err)
	}

	repo := repository.NewMovieRepository(gdb)
	svc := service.NewUploadService(repo, store, cfg.UploadService.PartSize, cfg.UploadService.PresignTTL)
	ctrl := controller.NewUploadController(svc)

	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		sqlDB, err := gdb.DB()
		if err == nil {
			err = sqlDB.Ping()
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	ctrl.RegisterRoutes(r)

	addr := ":" + cfg.UploadService.Port
	log.Printf("upload-service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("upload-service: %v", err)
	}
}
