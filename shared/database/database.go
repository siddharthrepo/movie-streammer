package database

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func DSN(host, port, user, password, name string) string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, host, port, name,
	)
}

func Connect(dsn string, maxOpen, maxIdle int, connMaxLifetime, slowThreshold time.Duration, quiet bool) (*gorm.DB, error) {
	level := logger.Info
	if quiet {
		level = logger.Warn
	}

	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newGormLogger(level, slowThreshold),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return gdb, nil
}
