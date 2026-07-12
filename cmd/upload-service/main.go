// Command upload-service is the HTTP entrypoint clients call to upload movies.
//
// This is currently a runnable skeleton: it starts the HTTP server and exposes
// a health check. The real endpoints (POST /uploads, POST /uploads/{id}/complete)
// and their wiring to config, Postgres, and MinIO are built next, function by
// function, per docs/features/01-upload-a-movie.md.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("UPLOAD_SERVICE_PORT")
	if port == "" {
		port = "8080"
	}

	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Printf("upload-service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("upload-service: %v", err)
	}
}
