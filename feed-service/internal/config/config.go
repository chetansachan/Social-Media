package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                  string
	FollowServiceAddr     string
	PostServiceAddr       string
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	TimelineMaxSize       int64
	TimelineTTLHours      int
	CelebrityThreshold    int32
	InactiveThresholdDays int
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found or error loading, using default/system environment variables")
	}

	port := getEnv("PORT", "50054")
	followAddr := getEnv("FOLLOW_SERVICE_ADDR", "localhost:50053")
	postAddr := getEnv("POST_SERVICE_ADDR", "localhost:50052")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := getEnvAsInt("REDIS_DB", 0)

	timelineMaxSize := getEnvAsInt64("TIMELINE_MAX_SIZE", 800)
	timelineTTLHours := getEnvAsInt("TIMELINE_TTL_HOURS", 720)       // 30 days default TTL
	celebrityThreshold := int32(getEnvAsInt("CELEBRITY_THRESHOLD", 25000))
	inactiveThresholdDays := getEnvAsInt("INACTIVE_THRESHOLD_DAYS", 14)

	return &Config{
		Port:                  port,
		FollowServiceAddr:     followAddr,
		PostServiceAddr:       postAddr,
		RedisAddr:             redisAddr,
		RedisPassword:         redisPassword,
		RedisDB:               redisDB,
		TimelineMaxSize:       timelineMaxSize,
		TimelineTTLHours:      timelineTTLHours,
		CelebrityThreshold:    celebrityThreshold,
		InactiveThresholdDays: inactiveThresholdDays,
	}
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func getEnvAsInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvAsInt64(key string, defaultVal int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultVal
	}
	return parsed
}
