package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port              string
	FollowServiceAddr string
	PostServiceAddr   string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found or error loading, using default/system environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "50054"
	}

	followAddr := os.Getenv("FOLLOW_SERVICE_ADDR")
	if followAddr == "" {
		followAddr = "localhost:50053"
	}

	postAddr := os.Getenv("POST_SERVICE_ADDR")
	if postAddr == "" {
		postAddr = "localhost:50052"
	}

	return &Config{
		Port:              port,
		FollowServiceAddr: followAddr,
		PostServiceAddr:   postAddr,
	}
}
