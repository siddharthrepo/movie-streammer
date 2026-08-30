package cmd

import (
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/shared/database"
	"github.com/siddharthraturi/movie-streamer/transcode-service/global"
)

func openDB() (*gorm.DB, error) {
	return database.Connect(
		database.DSN(global.MySQLHost, global.MySQLPort, global.MySQLUser, global.MySQLPassword, global.MySQLDatabase),
		global.MySQLMaxOpenConns,
		global.MySQLMaxIdleConns,
		global.MySQLConnMaxLifetime,
		global.SlowQueryThreshold,
		global.LogLevel != "debug",
	)
}
