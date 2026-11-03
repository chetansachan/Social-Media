package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	// DatabaseReadUrl is an optional read replica (usually a PgBouncer pool).
	// Empty means reads use DatabaseUrl.
	DatabaseReadUrl  string
	DatabaseMaxConns int32
	DatabaseMinConns int32

	Port      string
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
		Port:             os.Getenv("PORT"),
		SecretKey:        os.Getenv("MYSECRETKEY"),
	}

	return &config
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
