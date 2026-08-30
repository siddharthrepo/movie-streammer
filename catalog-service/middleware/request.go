package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/siddharthraturi/movie-streamer/shared/logger"
)

const (
	HeaderRequestID  = "X-Request-ID"
	ContextRequestID = "request_id"
	ContextLogger    = "logger"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(ContextRequestID, id)
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Set(ContextLogger, logger.L().With(zap.String("request_id", id)))
		c.Next()
	}
}

func From(c *gin.Context) *zap.Logger {
	if v, ok := c.Get(ContextLogger); ok {
		if l, ok := v.(*zap.Logger); ok {
			return l
		}
	}
	return logger.L()
}

func Access() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency_ms", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("bytes", c.Writer.Size()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		log := From(c)
		switch {
		case status >= 500:
			log.Error("request", fields...)
		case status >= 400:
			log.Warn("request", fields...)
		default:
			log.Info("request", fields...)
		}
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				From(c).Error("panic recovered",
					zap.Any("panic", err),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
