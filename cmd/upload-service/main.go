package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("upload-service: config: %v", err)
	}

	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	addr := ":" + cfg.UploadService.Port
	log.Printf("upload-service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("upload-service: %v", err)
	}
}
