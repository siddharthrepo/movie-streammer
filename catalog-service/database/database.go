package database

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/siddharthraturi/movie-streamer/catalog-service/global"
)

func Connect() (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		global.MySQLUser,
		global.MySQLPassword,
		global.MySQLHost,
		global.MySQLPort,
		global.MySQLDatabase,
	)

	level := logger.Info
	if global.GinMode == "release" {
		level = logger.Warn
	}

	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(level),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(global.MySQLMaxOpenConns)
	sqlDB.SetMaxIdleConns(global.MySQLMaxIdleConns)
	sqlDB.SetConnMaxLifetime(global.MySQLConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return gdb, nil
}
