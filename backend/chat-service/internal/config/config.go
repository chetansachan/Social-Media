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
	DatabaseUrl      string
	DatabaseReadUrl  string
	DatabaseMaxConns int32
	DatabaseMinConns int32
	Port             string
	// AdminPort serves /metrics, /healthz, and /readyz.
	AdminPort string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// EventStream is the Redis Stream every realtime frame is mirrored onto.
	// The gateway reads Pub/Sub for live sockets and the stream keeps the last
	// frames for a reconnecting client.
	EventStream string

	// PresenceTTL is how long a heartbeat keeps a user online without a new one.
	PresenceTTL time.Duration
	// TypingTTL is how long a typing signal survives, which makes a crashed
	// client stop looking busy on its own.
	TypingTTL time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading environment variables")
	}

	return &Config{
		DatabaseUrl:      os.Getenv("DATABASE_URL"),
		DatabaseReadUrl:  os.Getenv("DATABASE_READ_URL"),
		DatabaseMaxConns: int32(getEnvAsInt("DB_MAX_CONNS", 20)),
		DatabaseMinConns: int32(getEnvAsInt("DB_MIN_CONNS", 2)),
		Port:             os.Getenv("PORT"),
		AdminPort:        getEnv("ADMIN_PORT", "9097"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		EventStream: getEnv("CHAT_EVENT_STREAM", "social:events"),

		PresenceTTL: time.Duration(getEnvAsInt("PRESENCE_TTL_SECONDS", 60)) * time.Second,
		TypingTTL:   time.Duration(getEnvAsInt("TYPING_TTL_SECONDS", 6)) * time.Second,
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
