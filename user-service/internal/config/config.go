package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	Port        string
	SecretKey   string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("Error Loading Environment Variable : %v", err)
	}

	config := Config{
		DatabaseUrl: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
		SecretKey:   os.Getenv("MYSECRETKEY"),
	}

	return &config
}
