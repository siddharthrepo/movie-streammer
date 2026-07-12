package main

import (
	"context"
	"log"

	"github.com/siddharthraturi/movie-streamer/upload-service/controller"
	"github.com/siddharthraturi/movie-streamer/upload-service/database"
	"github.com/siddharthraturi/movie-streamer/upload-service/global"
	"github.com/siddharthraturi/movie-streamer/upload-service/repository"
	"github.com/siddharthraturi/movie-streamer/upload-service/route"
	"github.com/siddharthraturi/movie-streamer/upload-service/service"
	"github.com/siddharthraturi/movie-streamer/upload-service/storage"
)

func main() {
	gdb, err := database.Connect()
	if err != nil {
		log.Fatalf("upload-service: database: %v", err)
	}

	store, err := storage.NewMinIO()
	if err != nil {
		log.Fatalf("upload-service: storage: %v", err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		log.Fatalf("upload-service: ensure bucket: %v", err)
	}

	repo := repository.NewMovieRepository(gdb)
	svc := service.NewUploadService(repo, store)
	ctrl := controller.NewUploadController(svc)

	r := route.New(ctrl, gdb)

	addr := ":" + global.UploadServicePort
	log.Printf("upload-service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("upload-service: %v", err)
	}
}
