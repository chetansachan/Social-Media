package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	Port        string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading environment variables")
	}

	config := &Config{
		DatabaseUrl: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
	}

	return config
}
