package relay

import (
	"context"
	"fmt"
	"log"
	"time"

	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/kafka"
	"monolith/internal/models"
)

type Relay struct {
	repo     *database.Repository
	producer *kafka.Producer
	config   *config.RelayConfig
	running  bool
}

func NewRelay(repo *database.Repository, producer *kafka.Producer, cfg *config.RelayConfig) *Relay {
	return &Relay{
		repo:     repo,
		producer: producer,
		config:   cfg,
		running:  false,
	}
}

// Start begins the relay service polling loop
func (r *Relay) Start(ctx context.Context) error {
	if r.running {
		return fmt.Errorf("relay service is already running")
	}
	
	r.running = true
	log.Println("Starting outbox relay service...")
	
	// Reset any stale processing events on startup
	timeout := time.Duration(r.config.ProcessingTimeout) * time.Second
	if err := r.repo.ResetStaleProcessingEvents(timeout); err != nil {
		log.Printf("Warning: failed to reset stale processing events: %v", err)
	}
	
	ticker := time.NewTicker(time.Duration(r.config.PollInterval) * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Relay service shutting down...")
			r.running = false
			return ctx.Err()
		case <-ticker.C:
			if err := r.processOutboxEvents(); err != nil {
				log.Printf("Error processing outbox events: %v", err)
			}
		}
	}
}

// processOutboxEvents processes a batch of pending outbox events
func (r *Relay) processOutboxEvents() error {
	// Get pending events
	events, err := r.repo.GetPendingOutboxEvents(r.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending outbox events: %w", err)
	}
	
	if len(events) == 0 {
		return nil // No events to process
	}
	
	log.Printf("Processing %d outbox events", len(events))
	
	// Process each event
	for _, event := range events {
		if err := r.processEvent(event); err != nil {
			log.Printf("Failed to process event %s: %v", event.ID, err)
			// Continue processing other events even if one fails
		}
	}
	
	return nil
}

// processEvent processes a single outbox event
func (r *Relay) processEvent(event *models.OutboxEvent) error {
	// Check if event has exceeded max retries
	if event.RetryCount >= event.MaxRetries {
		log.Printf("Event %s has exceeded max retries (%d), marking as failed",
			event.ID, event.MaxRetries)
		return r.markEventAsFailed(event, "exceeded maximum retry attempts")
	}
	
	// Start transaction to mark event as processing
	tx, err := r.repo.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	// Mark event as processing (this prevents other relay instances from processing it)
	if err := r.repo.MarkEventAsProcessing(tx, event.ID); err != nil {
		// Event might already be processed by another instance
		return fmt.Errorf("failed to mark event as processing: %w", err)
	}
	
	// Commit the processing status
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit processing status: %w", err)
	}
	
	// Publish to Kafka
	if err := r.producer.PublishEvent(event); err != nil {
		// Mark as failed and increment retry count
		return r.markEventAsFailed(event, err.Error())
	}
	
	// Mark as sent
	return r.markEventAsSent(event)
}

// markEventAsSent marks an event as successfully sent
func (r *Relay) markEventAsSent(event *models.OutboxEvent) error {
	tx, err := r.repo.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	if err := r.repo.MarkEventAsSent(tx, event.ID); err != nil {
		return fmt.Errorf("failed to mark event as sent: %w", err)
	}
	
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit sent status: %w", err)
	}
	
	log.Printf("Successfully processed event %s (type: %s)", event.ID, event.EventType)
	return nil
}

// markEventAsFailed marks an event as failed and increments retry count
func (r *Relay) markEventAsFailed(event *models.OutboxEvent, errorMsg string) error {
	tx, err := r.repo.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	if err := r.repo.MarkEventAsFailed(tx, event.ID, errorMsg); err != nil {
		return fmt.Errorf("failed to mark event as failed: %w", err)
	}
	
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit failed status: %w", err)
	}
	
	log.Printf("Marked event %s as failed: %s", event.ID, errorMsg)
	return nil
}

// ProcessSingleEvent processes a single event by ID (useful for testing)
func (r *Relay) ProcessSingleEvent(eventID string) error {
	// This would require additional repository method to get event by ID
	// For now, we'll just trigger a processing cycle
	return r.processOutboxEvents()
}

// GetStats returns relay processing statistics
func (r *Relay) GetStats() (*RelayStats, error) {
	// This would require additional repository methods to get counts
	// For now, return basic stats
	return &RelayStats{
		Running:     r.running,
		LastRun:     time.Now(), // Would track actual last run time
		BatchSize:   r.config.BatchSize,
		MaxRetries:  r.config.MaxRetries,
		PollInterval: r.config.PollInterval,
	}, nil
}

type RelayStats struct {
	Running      bool      `json:"running"`
	LastRun      time.Time `json:"last_run"`
	BatchSize    int       `json:"batch_size"`
	MaxRetries   int       `json:"max_retries"`
	PollInterval int       `json:"poll_interval"`
}

// IsRunning returns whether the relay service is currently running
func (r *Relay) IsRunning() bool {
	return r.running
}

// Stop stops the relay service (requires context cancellation in Start)
func (r *Relay) Stop() {
	r.running = false
}
