package main

import (
	"log"
	"net/http"
	"task-api/internal/api"
	"task-api/internal/config"
	"task-api/internal/queue"
	"task-api/internal/storage"

	"github.com/gorilla/mux"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Init Storage
	store, err := storage.New(cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Init RabbitMQ
	q, err := queue.New(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer q.Close()

	// Init Handlers
	handler := api.NewHandler(store, q)
	r := mux.NewRouter()
	handler.RegisterRoutes(r)

	log.Printf("Starting server on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
