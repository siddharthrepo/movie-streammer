package cmd

import (
	"gorm.io/gorm"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
	"github.com/siddharthraturi/movie-streamer/shared/database"
)

func openDB() (*gorm.DB, error) {
	return database.Connect(
		database.DSN(global.MySQLHost, global.MySQLPort, global.MySQLUser, global.MySQLPassword, global.MySQLDatabase),
		global.MySQLMaxOpenConns,
		global.MySQLMaxIdleConns,
		global.MySQLConnMaxLifetime,
		global.SlowQueryThreshold,
		global.GinMode == "release",
	)
}
