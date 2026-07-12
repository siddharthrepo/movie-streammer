package global

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

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		UploadService: UploadServiceConfig{
			Port:       getEnv(EnvUploadServicePort, DefaultUploadServicePort),
			PartSize:   int64(getEnvInt(EnvUploadPartSize, DefaultUploadPartSize)),
			PresignTTL: time.Duration(getEnvInt(EnvUploadPresignTTLSec, DefaultUploadPresignTTLSec)) * time.Second,
		},
		Postgres: PostgresConfig{
			Host:     getEnv(EnvPostgresHost, DefaultPostgresHost),
			Port:     getEnv(EnvPostgresPort, DefaultPostgresPort),
			User:     getEnv(EnvPostgresUser, DefaultPostgresUser),
			Password: getEnv(EnvPostgresPassword, DefaultPostgresPassword),
			DB:       getEnv(EnvPostgresDB, DefaultPostgresDB),
			SSLMode:  getEnv(EnvPostgresSSLMode, DefaultPostgresSSLMode),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv(EnvMinIOEndpoint, DefaultMinIOEndpoint),
			AccessKey: getEnv(EnvMinIOAccessKey, DefaultMinIOAccessKey),
			SecretKey: getEnv(EnvMinIOSecretKey, DefaultMinIOSecretKey),
			UseSSL:    getEnvBool(EnvMinIOUseSSL, false),
			Bucket:    getEnv(EnvMinIOBucket, DefaultMinIOBucket),
		},
	}

	if cfg.UploadService.PartSize < MinPartSize {
		return nil, fmt.Errorf("%s must be >= 5MB, got %d", EnvUploadPartSize, cfg.UploadService.PartSize)
	}
	if cfg.MinIO.Bucket == "" {
		return nil, fmt.Errorf("%s must not be empty", EnvMinIOBucket)
	}
	return cfg, nil
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

type Movie struct {
	ID          string `gorm:"type:uuid;default:gen_random_uuid()"`
	Filename    string
	ObjectKey   string
	SizeBytes   int64
	ContentType string
	Status      string
	UploadID    string
	PartSize    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Movie) TableName() string { return "movies" }

type Part struct {
	Number int
	ETag   string
	Size   int64
}

type InitResult struct {
	MovieID   string
	UploadID  string
	PartSize  int64
	PartCount int
}

type StatusResult struct {
	Status        string
	PartSize      int64
	PartCount     int
	UploadedParts []int
}

type InitUploadRequest struct {
	Filename    string `json:"filename" binding:"required"`
	Size        int64  `json:"size" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

type PresignPartsRequest struct {
	PartNumbers []int `json:"part_numbers" binding:"required"`
}
