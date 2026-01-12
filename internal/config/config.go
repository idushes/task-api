package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	PostgresURL string
	RabbitMQURL string
	Port        string
}

func Load() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Only log if it's a specific error or maybe just ignore "file not found"
		// For simplicity, we can ignore the error, assuming the user might configure via real ENV vars.
		// However, it's often helpful to know if .env failed to load for other reasons.
		// But valid use case: no .env file in prod.
		// We can check if the error is os.ErrNotExist logic, but godotenv returns its own error.
		// Let's just swallow the error or print a debug log?
		// Standard practice: Try to load, if fail, proceed.
	}

	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		return nil, fmt.Errorf("POSTGRES_URL environment variable is not set")
	}

	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL environment variable is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		PostgresURL: pgURL,
		RabbitMQURL: rabbitURL,
		Port:        port,
	}, nil
}
