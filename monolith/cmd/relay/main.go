package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/kafka"
	"monolith/internal/relay"
)

func main() {
	log.Println("Starting Outbox Relay Service...")

	cfg := config.Load()

	// Connect to database
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	repo := database.NewRepository(db)

	// Create Kafka producer
	producer, err := kafka.NewProducer(&kafka.ProducerConfig{
		Brokers:  cfg.Kafka.Brokers,
		ClientID: "outbox-relay",
	})
	if err != nil {
		log.Fatalf("Failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	// Health check
	if err := producer.HealthCheck(); err != nil {
		log.Fatalf("Kafka health check failed: %v", err)
	}

	// Create relay service
	relayService := relay.NewRelay(repo, producer, &cfg.Relay)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start relay service in a goroutine
	go func() {
		if err := relayService.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("Relay service error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Received shutdown signal, gracefully shutting down...")

	// Cancel context to stop relay service
	cancel()

	log.Println("Relay service stopped")
}
