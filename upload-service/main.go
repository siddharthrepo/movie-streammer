package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/siddharthraturi/movie-streamer/shared/config"
	"github.com/siddharthraturi/movie-streamer/shared/database"
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

	addr := ":" + cfg.UploadService.Port
	log.Printf("upload-service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("upload-service: %v", err)
	}
}
