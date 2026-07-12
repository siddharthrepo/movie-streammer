package route

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/upload-service/controller"
)

func New(uc *controller.UploadController, gdb *gorm.DB) *gin.Engine {
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

	uploads := r.Group("/uploads")
	uploads.POST("", uc.InitUpload)
	uploads.POST("/:id/parts/urls", uc.PresignParts)
	uploads.GET("/:id/status", uc.Status)
	uploads.POST("/:id/complete", uc.Complete)
	uploads.POST("/:id/abort", uc.Abort)

	return r
}
