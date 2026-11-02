package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every runtime knob used by the Feed Service.
type Config struct {
	Port              string
	FollowServiceAddr string
	PostServiceAddr   string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	TimelineMaxSize       int64
	TimelineTTL           time.Duration
	CelebrityThreshold    int32
	InactiveThresholdDays int

	EventConsumerEnabled bool
	EventStream          string
	EventGroup           string
	EventConsumerName    string
}

// Load reads configuration from .env (when present) and the process
// environment. Defaults are production-safe and match the system design.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found or error loading, using environment variables and defaults")
	}

	return &Config{
		Port:              getEnv("PORT", "50054"),
		FollowServiceAddr: getEnv("FOLLOW_SERVICE_ADDR", "localhost:50053"),
		PostServiceAddr:   getEnv("POST_SERVICE_ADDR", "localhost:50052"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		TimelineMaxSize:       getEnvAsInt64("TIMELINE_MAX_SIZE", 800),
		TimelineTTL:           time.Duration(getEnvAsInt("TIMELINE_TTL_HOURS", 720)) * time.Hour,
		CelebrityThreshold:    int32(getEnvAsInt("CELEBRITY_THRESHOLD", 25000)),
		InactiveThresholdDays: getEnvAsInt("INACTIVE_THRESHOLD_DAYS", 14),

		EventConsumerEnabled: getEnvAsBool("EVENT_CONSUMER_ENABLED", true),
		EventStream:          getEnv("EVENT_STREAM", "social:events"),
		EventGroup:           getEnv("EVENT_GROUP", "feed-service"),
		EventConsumerName:    getEnv("EVENT_CONSUMER_NAME", defaultConsumerName()),
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

func getEnvAsInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultConsumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "feed-service"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
