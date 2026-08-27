package route

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/controller"
	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/catalog-service/logger"
	"github.com/siddharthraturi/movie-streamer/catalog-service/middleware"
)

func New(movieCtrl *controller.MovieController, gdb *gorm.DB) *gin.Engine {
	gin.SetMode(global.GinMode)
	gin.DefaultWriter = io.Discard
	gin.DebugPrintRouteFunc = func(method, path, handler string, n int) {
		logger.L().Debug("route", zap.String("method", method), zap.String("path", path))
	}

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Access(), middleware.Recovery())

	r.GET("/healthz", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(ctx *gin.Context) {
		sqlDB, err := gdb.DB()
		if err != nil || sqlDB.PingContext(ctx.Request.Context()) != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unreachable"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	v1 := r.Group("/api/v1")
	{
		movies := v1.Group("/movies")
		{
			movies.POST("", movieCtrl.Create)
			movies.GET("", movieCtrl.List)
			movies.GET("/:id", movieCtrl.Get)
			movies.PATCH("/:id", movieCtrl.Update)
			movies.DELETE("/:id", movieCtrl.Delete)
		}
	}

	return r
}
