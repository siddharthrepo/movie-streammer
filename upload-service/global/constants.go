package global

const (
	MaxParts    = 10000
	MinPartSize = 5 * 1024 * 1024
)

const (
	EnvUploadServicePort   = "UPLOAD_SERVICE_PORT"
	EnvUploadPartSize      = "UPLOAD_PART_SIZE"
	EnvUploadPresignTTLSec = "UPLOAD_PRESIGN_TTL_SECONDS"
	EnvPostgresHost        = "POSTGRES_HOST"
	EnvPostgresPort        = "POSTGRES_PORT"
	EnvPostgresUser        = "POSTGRES_USER"
	EnvPostgresPassword    = "POSTGRES_PASSWORD"
	EnvPostgresDB          = "POSTGRES_DB"
	EnvPostgresSSLMode     = "POSTGRES_SSLMODE"
	EnvMinIOEndpoint       = "MINIO_ENDPOINT"
	EnvMinIOAccessKey      = "MINIO_ACCESS_KEY"
	EnvMinIOSecretKey      = "MINIO_SECRET_KEY"
	EnvMinIOUseSSL         = "MINIO_USE_SSL"
	EnvMinIOBucket         = "MINIO_BUCKET"
)

const (
	DefaultUploadServicePort   = "8080"
	DefaultUploadPartSize      = 52428800
	DefaultUploadPresignTTLSec = 3600
	DefaultPostgresHost        = "localhost"
	DefaultPostgresPort        = "5544"
	DefaultPostgresUser        = "movie"
	DefaultPostgresPassword    = "movie"
	DefaultPostgresDB          = "movie_streamer"
	DefaultPostgresSSLMode     = "disable"
	DefaultMinIOEndpoint       = "localhost:9000"
	DefaultMinIOAccessKey      = "minioadmin"
	DefaultMinIOSecretKey      = "minioadmin"
	DefaultMinIOBucket         = "movies"
)
