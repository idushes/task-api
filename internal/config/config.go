package config

import (
	"fmt"
	"os"
)

type Config struct {
	PostgresURL string
	RabbitMQURL string
	Port        string
}

func Load() (*Config, error) {
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
