package database

import (
	"time"

	"gorm.io/gorm/logger"

	applog "github.com/siddharthraturi/movie-streamer/shared/logger"
)

type zapWriter struct{}

func (zapWriter) Printf(format string, args ...any) {
	applog.S().Debugf(format, args...)
}

func newGormLogger(level logger.LogLevel, slowThreshold time.Duration) logger.Interface {
	return logger.New(zapWriter{}, logger.Config{
		SlowThreshold:             slowThreshold,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      false,
		Colorful:                  false,
	})
}
