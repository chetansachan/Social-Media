package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every runtime knob used by the Notification Service. Values are
// read from the process environment, optionally seeded from a local .env file.
type Config struct {
	Port   string
	WSPort string
	// AdminPort serves /metrics, /healthz, and /readyz.
	AdminPort string
	Env       string

	DatabaseURL       string
	DatabaseReadURL   string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	EventConsumerEnabled bool
	EventStreams         []string
	EventGroup           string
	EventConsumerName    string
	EventClaimInterval   time.Duration
	EventMaxAttempts     int
	EventDLQStream       string

	UserServiceAddr string

	AggregationWindow time.Duration
	DedupTTL          time.Duration
	RealtimeThrottle  time.Duration
	MaxPushPerHour    int
	PushRateWindow    time.Duration
	UnreadCacheTTL    time.Duration
	RetentionDays     int

	JWTSecret      string
	AllowDevUserID bool
	AllowedOrigins []string

	PushProvider       string
	FCMCredentialsFile string
	FCMProjectID       string
	APNSAuthKeyFile    string
	APNSKeyID          string
	APNSTeamID         string
	APNSTopic          string
	APNSSandbox        bool

	EmailProvider string
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	EmailFrom     string

	WorkerCount int
	QueueSize   int
}

// Load reads configuration from .env (when present) and the process
// environment. Defaults match the local development topology described in
// README.md and SYSTEM_DESIGN.md.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found or error loading, using environment variables and defaults")
	}

	env := getEnv("ENV", "development")

	return &Config{
		Port:      getEnv("PORT", "50055"),
		WSPort:    getEnv("WS_PORT", "8085"),
		AdminPort: getEnv("ADMIN_PORT", "9095"),
		Env:       env,

		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/socialflow_notifications?sslmode=disable"),
		DatabaseReadURL:   os.Getenv("DATABASE_READ_URL"),
		DBMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime: getEnvAsDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		EventConsumerEnabled: getEnvAsBool("EVENT_CONSUMER_ENABLED", true),
		EventStreams:         getEnvAsList("EVENT_STREAM", []string{"social:events"}),
		EventGroup:           getEnv("EVENT_GROUP", "notification-service"),
		EventConsumerName:    getEnv("EVENT_CONSUMER_NAME", defaultConsumerName()),
		EventClaimInterval:   getEnvAsDuration("EVENT_CLAIM_INTERVAL", 30*time.Second),
		EventMaxAttempts:     getEnvAsInt("EVENT_MAX_ATTEMPTS", 5),
		EventDLQStream:       getEnv("EVENT_DLQ_STREAM", "stream:notifications:dlq"),

		UserServiceAddr: getEnv("USER_SERVICE_ADDR", "localhost:50051"),

		AggregationWindow: getEnvAsDuration("AGGREGATION_WINDOW", 15*time.Minute),
		DedupTTL:          getEnvAsDuration("DEDUP_TTL", 60*time.Second),
		RealtimeThrottle:  getEnvAsDuration("REALTIME_THROTTLE", 30*time.Second),
		MaxPushPerHour:    getEnvAsInt("MAX_PUSH_PER_HOUR", 5),
		PushRateWindow:    getEnvAsDuration("PUSH_RATE_WINDOW", time.Hour),
		UnreadCacheTTL:    getEnvAsDuration("UNREAD_CACHE_TTL", 7*24*time.Hour),
		RetentionDays:     getEnvAsInt("RETENTION_DAYS", 90),

		JWTSecret:      os.Getenv("JWT_SECRET"),
		AllowDevUserID: getEnvAsBool("ALLOW_DEV_USER_ID", env != "production"),
		AllowedOrigins: getEnvAsList("ALLOWED_ORIGINS", []string{"*"}),

		PushProvider:       strings.ToLower(getEnv("PUSH_PROVIDER", "log")),
		FCMCredentialsFile: getEnv("FCM_CREDENTIALS_FILE", ""),
		FCMProjectID:       getEnv("FCM_PROJECT_ID", ""),
		APNSAuthKeyFile:    getEnv("APNS_KEY_FILE", ""),
		APNSKeyID:          getEnv("APNS_KEY_ID", ""),
		APNSTeamID:         getEnv("APNS_TEAM_ID", ""),
		APNSTopic:          getEnv("APNS_TOPIC", "com.socialflow.app"),
		APNSSandbox:        getEnvAsBool("APNS_SANDBOX", false),

		EmailProvider: strings.ToLower(getEnv("EMAIL_PROVIDER", "log")),
		SMTPHost:      getEnv("SMTP_HOST", ""),
		SMTPPort:      getEnvAsInt("SMTP_PORT", 587),
		SMTPUsername:  getEnv("SMTP_USERNAME", ""),
		SMTPPassword:  os.Getenv("SMTP_PASSWORD"),
		EmailFrom:     getEnv("EMAIL_FROM", "notifications@socialflow.local"),

		WorkerCount: getEnvAsInt("WORKER_COUNT", 8),
		QueueSize:   getEnvAsInt("QUEUE_SIZE", 1024),
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

// getEnvAsDuration accepts either a Go duration ("30s", "15m") or a plain
// integer number of seconds ("30").
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

func getEnvAsList(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func defaultConsumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "notification-service"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
