package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	UploadService UploadServiceConfig
	Postgres      PostgresConfig
	MinIO         MinIOConfig
}

type UploadServiceConfig struct {
	Port       string
	PartSize   int64
	PresignTTL time.Duration
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.DB, p.SSLMode,
	)
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		UploadService: UploadServiceConfig{
			Port:       getEnv("UPLOAD_SERVICE_PORT", "8080"),
			PartSize:   int64(getEnvInt("UPLOAD_PART_SIZE", 52428800)),
			PresignTTL: time.Duration(getEnvInt("UPLOAD_PRESIGN_TTL_SECONDS", 3600)) * time.Second,
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5544"),
			User:     getEnv("POSTGRES_USER", "movie"),
			Password: getEnv("POSTGRES_PASSWORD", "movie"),
			DB:       getEnv("POSTGRES_DB", "movie_streamer"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:    getEnvBool("MINIO_USE_SSL", false),
			Bucket:    getEnv("MINIO_BUCKET", "movies"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.UploadService.PartSize < 5*1024*1024 {
		return fmt.Errorf("UPLOAD_PART_SIZE must be >= 5MB (S3 multipart minimum), got %d", c.UploadService.PartSize)
	}
	if c.MinIO.Bucket == "" {
		return fmt.Errorf("MINIO_BUCKET must not be empty")
	}
	return nil
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
