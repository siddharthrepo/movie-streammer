package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	UploadService UploadServiceConfig
	Postgres      PostgresConfig
	MinIO         MinIOConfig
	RabbitMQ      RabbitMQConfig
	Transcode     TranscodeConfig
}

type UploadServiceConfig struct {
	Port string
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

type RabbitMQConfig struct {
	URL string
}

type TranscodeConfig struct {
	MaxAttempts int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		UploadService: UploadServiceConfig{
			Port: getEnv("UPLOAD_SERVICE_PORT", "8080"),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
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
		RabbitMQ: RabbitMQConfig{
			URL: getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		},
		Transcode: TranscodeConfig{
			MaxAttempts: getEnvInt("TRANSCODE_MAX_ATTEMPTS", 3),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Transcode.MaxAttempts < 1 {
		return fmt.Errorf("TRANSCODE_MAX_ATTEMPTS must be >= 1, got %d", c.Transcode.MaxAttempts)
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
