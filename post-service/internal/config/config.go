package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseUrl string
	Port        string
	CloudinaryUrl string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error Loading environment variables")
	}

	config := &Config{
		DatabaseUrl: os.Getenv("DATABASE_URL"),
		Port: os.Getenv("PORT"),
		CloudinaryUrl: os.Getenv("CLOUDINARY_URL"),
	}

	return config
}