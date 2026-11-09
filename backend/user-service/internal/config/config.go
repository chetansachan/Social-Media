package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	// DatabaseReadUrl is an optional read replica (usually a PgBouncer pool).
	// Empty means reads use DatabaseUrl.
	DatabaseReadUrl  string
	DatabaseMaxConns int32
	DatabaseMinConns int32

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	// UserCacheTTL bounds how long a public profile may be served from Redis
	// without consulting PostgreSQL.
	UserCacheTTL time.Duration

	Port string
	// AdminPort serves /metrics, /healthz, and /readyz on the pod network.
	AdminPort string
	SecretKey string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("Error Loading Environment Variable : %v", err)
	}

	config := Config{
		DatabaseUrl:      os.Getenv("DATABASE_URL"),
		DatabaseReadUrl:  os.Getenv("DATABASE_READ_URL"),
		DatabaseMaxConns: int32(envAsInt("DB_MAX_CONNS", 20)),
		DatabaseMinConns: int32(envAsInt("DB_MIN_CONNS", 2)),

		RedisAddr:     envOr("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       envAsInt("REDIS_DB", 0),
		UserCacheTTL:  envAsDuration("USER_CACHE_TTL", 10*time.Minute),

		Port:      os.Getenv("PORT"),
		AdminPort: envOr("ADMIN_PORT", "9091"),
		SecretKey: os.Getenv("MYSECRETKEY"),
	}

	return &config
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envAsDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envAsInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
