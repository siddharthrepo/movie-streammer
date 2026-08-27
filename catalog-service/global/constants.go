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

	JobPendingUpload = "pending_upload"
	JobUploaded      = "uploaded"
	JobProcessing    = "processing"
	JobCompleted     = "completed"
	JobFailed        = "failed"

	DefaultPageSize = 20
	MaxPageSize     = 100

	MaxParts    = 10000
	MinPartSize = 5 * 1024 * 1024
	MaxFileSize = 100 * 1024 * 1024 * 1024

	LocalStackEndpoint  = "http://localhost:4576"
	LocalStackAccessKey = "test"
	LocalStackSecretKey = "test"
)

var (
	CatalogServicePort string
	GinMode            string
	ShutdownTimeout    time.Duration

	LogLevel           string
	LogFormat          string
	SlowQueryThreshold time.Duration

	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
	S3IsLocal      bool

	UploadPartSize   int64
	UploadPresignTTL time.Duration

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

	LogLevel = getEnv("LOG_LEVEL", "info")
	LogFormat = getEnv("LOG_FORMAT", "console")
	SlowQueryThreshold = time.Duration(getEnvInt("SLOW_QUERY_THRESHOLD_MS", 200)) * time.Millisecond

	S3Region = getEnv("S3_REGION", "us-east-1")
	S3Bucket = getEnv("S3_BUCKET", "movie-streamer-media")
	S3AccessKey = getEnv("S3_ACCESS_KEY", "")
	S3SecretKey = getEnv("S3_SECRET_KEY", "")
	S3Endpoint = getEnv("S3_ENDPOINT", "")

	S3IsLocal = S3AccessKey == "" || S3SecretKey == ""
	if S3IsLocal {
		S3AccessKey = LocalStackAccessKey
		S3SecretKey = LocalStackSecretKey
		if S3Endpoint == "" {
			S3Endpoint = LocalStackEndpoint
		}
	}
	S3UsePathStyle = getEnvBool("S3_USE_PATH_STYLE", S3IsLocal)

	UploadPartSize = int64(getEnvInt("UPLOAD_PART_SIZE_BYTES", 64*1024*1024))
	UploadPresignTTL = time.Duration(getEnvInt("UPLOAD_PRESIGN_TTL_SECONDS", 3600)) * time.Second

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

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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
