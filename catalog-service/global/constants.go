package global

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	StatusDraft  = "draft"
	StatusReady  = "ready"
	StatusFailed = "failed"

	DefaultPageSize = 20
	MaxPageSize     = 100
)

var (
	CatalogServicePort string
	GinMode            string
	ShutdownTimeout    time.Duration

	MySQLHost     string
	MySQLPort     string
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string

	MySQLMaxOpenConns    int
	MySQLMaxIdleConns    int
	MySQLConnMaxLifetime time.Duration
)

func init() {
	_ = godotenv.Load()

	CatalogServicePort = getEnv("CATALOG_SERVICE_PORT", "8081")
	GinMode = getEnv("GIN_MODE", "debug")
	ShutdownTimeout = time.Duration(getEnvInt("SHUTDOWN_TIMEOUT_SECONDS", 15)) * time.Second

	MySQLHost = getEnv("MYSQL_HOST", "localhost")
	MySQLPort = getEnv("MYSQL_PORT", "3306")
	MySQLUser = getEnv("MYSQL_USER", "movie")
	MySQLPassword = getEnv("MYSQL_PASSWORD", "movie")
	MySQLDatabase = getEnv("MYSQL_DATABASE", "movie_streamer")

	MySQLMaxOpenConns = getEnvInt("MYSQL_MAX_OPEN_CONNS", 25)
	MySQLMaxIdleConns = getEnvInt("MYSQL_MAX_IDLE_CONNS", 5)
	MySQLConnMaxLifetime = time.Duration(getEnvInt("MYSQL_CONN_MAX_LIFETIME_MINUTES", 30)) * time.Minute
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
