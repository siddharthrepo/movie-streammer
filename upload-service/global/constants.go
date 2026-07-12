package global

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const MaxParts = 10000

var (
	UploadServicePort string
	UploadPartSize    int64
	UploadPresignTTL  int

	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOUseSSL    bool
	MinIOBucket    string
)

func init() {
	_ = godotenv.Load()

	UploadServicePort = getEnv("UPLOAD_SERVICE_PORT", "8080")
	UploadPartSize = int64(getEnvInt("UPLOAD_PART_SIZE", 52428800))
	UploadPresignTTL = getEnvInt("UPLOAD_PRESIGN_TTL_SECONDS", 3600)

	PostgresHost = getEnv("POSTGRES_HOST", "localhost")
	PostgresPort = getEnv("POSTGRES_PORT", "5544")
	PostgresUser = getEnv("POSTGRES_USER", "movie")
	PostgresPassword = getEnv("POSTGRES_PASSWORD", "movie")
	PostgresDB = getEnv("POSTGRES_DB", "movie_streamer")
	PostgresSSLMode = getEnv("POSTGRES_SSLMODE", "disable")

	MinIOEndpoint = getEnv("MINIO_ENDPOINT", "localhost:9000")
	MinIOAccessKey = getEnv("MINIO_ACCESS_KEY", "minioadmin")
	MinIOSecretKey = getEnv("MINIO_SECRET_KEY", "minioadmin")
	MinIOUseSSL = getEnvBool("MINIO_USE_SSL", false)
	MinIOBucket = getEnv("MINIO_BUCKET", "movies")
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
