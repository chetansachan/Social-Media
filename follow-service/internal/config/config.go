package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	Port        string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

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
		fmt.Println("Error loading environment variables")
	}

	return &Config{
		DatabaseUrl: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

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
