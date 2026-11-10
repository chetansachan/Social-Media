package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// The defaults match the composer's client-side caps. Byte counts keep this
// package independent of the services that read them.
const (
	// DefaultMediaImageMaxBytes is the cap on a still image or GIF.
	DefaultMediaImageMaxBytes = 10 * 1024 * 1024
	// DefaultMediaVideoMaxBytes is the cap on a video.
	DefaultMediaVideoMaxBytes = 64 * 1024 * 1024
)

type Config struct {
	DatabaseUrl      string
	DatabaseReadUrl  string
	DatabaseMaxConns int32
	DatabaseMinConns int32
	Port             string
	// AdminPort serves /metrics, /healthz, and /readyz.
	AdminPort     string
	CloudinaryUrl string

	// MediaImageMaxBytes and MediaVideoMaxBytes cap how much a caller may hand
	// to Upload. They mirror what the composer enforces, so a direct API caller
	// hits the same wall instead of a silently different one. A URL upload is
	// not capped here: its size is only knowable after fetching it.
	MediaImageMaxBytes int64
	MediaVideoMaxBytes int64

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	// PostCacheTTL bounds how long a post detail may be served from Redis
	// without consulting PostgreSQL.
	PostCacheTTL time.Duration

	EventStream           string
	OutboxPollInterval    time.Duration
	OutboxBatchSize       int
	OutboxMaxAttempts     int
	OutboxStaleAfter      time.Duration
	OutboxPruneAfter      time.Duration
	OutboxReclaimInterval time.Duration
	OutboxBaseBackoff     time.Duration
	OutboxMaxBackoff      time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error Loading environment variables")
	}

	return &Config{
		DatabaseUrl:        os.Getenv("DATABASE_URL"),
		DatabaseReadUrl:    os.Getenv("DATABASE_READ_URL"),
		DatabaseMaxConns:   int32(getEnvAsInt("DB_MAX_CONNS", 20)),
		DatabaseMinConns:   int32(getEnvAsInt("DB_MIN_CONNS", 2)),
		Port:               os.Getenv("PORT"),
		AdminPort:          getEnv("ADMIN_PORT", "9092"),
		CloudinaryUrl:      os.Getenv("CLOUDINARY_URL"),
		MediaImageMaxBytes: int64(getEnvAsInt("MEDIA_IMAGE_MAX_BYTES", DefaultMediaImageMaxBytes)),
		MediaVideoMaxBytes: int64(getEnvAsInt("MEDIA_VIDEO_MAX_BYTES", DefaultMediaVideoMaxBytes)),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),
		PostCacheTTL:  getEnvAsDuration("POST_CACHE_TTL", 5*time.Minute),

		EventStream:           getEnv("EVENT_STREAM", "social:events"),
		OutboxPollInterval:    getEnvAsDuration("OUTBOX_POLL_INTERVAL", 500*time.Millisecond),
		OutboxBatchSize:       getEnvAsInt("OUTBOX_BATCH_SIZE", 100),
		OutboxMaxAttempts:     getEnvAsInt("OUTBOX_MAX_ATTEMPTS", 10),
		OutboxStaleAfter:      getEnvAsDuration("OUTBOX_STALE_AFTER", 2*time.Minute),
		OutboxPruneAfter:      getEnvAsDuration("OUTBOX_PRUNE_AFTER", 7*24*time.Hour),
		OutboxReclaimInterval: getEnvAsDuration("OUTBOX_RECLAIM_INTERVAL", time.Minute),
		OutboxBaseBackoff:     getEnvAsDuration("OUTBOX_BASE_BACKOFF", 2*time.Second),
		OutboxMaxBackoff:      getEnvAsDuration("OUTBOX_MAX_BACKOFF", 5*time.Minute),
	}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
