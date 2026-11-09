package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
		AdminPort:        getEnv("ADMIN_PORT", "9096"),
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
