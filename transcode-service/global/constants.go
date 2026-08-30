package global

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	ChunkPending = "pending"
	ChunkLeased  = "leased"
	ChunkDone    = "done"
	ChunkFailed  = "failed"

	JobProcessing = "processing"
	JobCompleted  = "completed"
	JobFailed     = "failed"
)

var (
	LogLevel           string
	LogFormat          string
	SlowQueryThreshold time.Duration

	WorkerConcurrency  int
	WorkerBatchSize    int
	WorkerPollInterval time.Duration
	WorkerIdleBackoff  time.Duration
	WorkerMaxAttempts  int

	LeaseTTL             time.Duration
	LeaseRenewInterval   time.Duration
	LeaseReclaimGrace    time.Duration
	ShutdownDrainTimeout time.Duration

	FakeWorkDuration time.Duration
	FakeFailPercent  int

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

	LogLevel = getEnv("LOG_LEVEL", "info")
	LogFormat = getEnv("LOG_FORMAT", "console")
	SlowQueryThreshold = time.Duration(getEnvInt("SLOW_QUERY_THRESHOLD_MS", 200)) * time.Millisecond

	WorkerConcurrency = getEnvInt("WORKER_CONCURRENCY", 3)
	WorkerBatchSize = getEnvInt("WORKER_BATCH_SIZE", 1)
	WorkerPollInterval = time.Duration(getEnvInt("WORKER_POLL_INTERVAL_MS", 500)) * time.Millisecond
	WorkerIdleBackoff = time.Duration(getEnvInt("WORKER_IDLE_BACKOFF_MS", 3000)) * time.Millisecond
	WorkerMaxAttempts = getEnvInt("WORKER_MAX_ATTEMPTS", 3)

	LeaseTTL = time.Duration(getEnvInt("LEASE_TTL_SECONDS", 60)) * time.Second
	LeaseRenewInterval = time.Duration(getEnvInt("LEASE_RENEW_INTERVAL_SECONDS", 20)) * time.Second
	LeaseReclaimGrace = time.Duration(getEnvInt("LEASE_RECLAIM_GRACE_SECONDS", 5)) * time.Second
	ShutdownDrainTimeout = time.Duration(getEnvInt("SHUTDOWN_DRAIN_TIMEOUT_SECONDS", 30)) * time.Second

	FakeWorkDuration = time.Duration(getEnvInt("FAKE_WORK_MS", 2000)) * time.Millisecond
	FakeFailPercent = getEnvInt("FAKE_FAIL_PERCENT", 0)

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
