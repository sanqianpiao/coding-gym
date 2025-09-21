package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/kafka"
	"monolith/internal/models"
)

type BackfillOptions struct {
	startTime     time.Time
	endTime       time.Time
	aggregateType string
	dryRun        bool
	batchSize     int
	maxRetries    int
	topic         string
}

func main() {
	var (
		startTimeStr  = flag.String("start", "", "Start time for backfill (RFC3339 format)")
		endTimeStr    = flag.String("end", "", "End time for backfill (RFC3339 format)")
		aggregateType = flag.String("aggregate-type", "", "Aggregate type to backfill")
		dryRun        = flag.Bool("dry-run", false, "Perform a dry run without actually publishing")
		batchSize     = flag.Int("batch-size", 100, "Number of events to process in each batch")
		maxRetries    = flag.Int("max-retries", 3, "Maximum number of retry attempts")
		topic         = flag.String("topic", "", "Override topic for backfill events")
		help          = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	cfg := config.Load()

	// Parse time parameters
	opts := BackfillOptions{
		dryRun:        *dryRun,
		batchSize:     *batchSize,
		maxRetries:    *maxRetries,
		aggregateType: *aggregateType,
		topic:         *topic,
	}

	if *startTimeStr != "" {
		var err error
		opts.startTime, err = time.Parse(time.RFC3339, *startTimeStr)
		if err != nil {
			log.Fatalf("Invalid start time format: %v", err)
		}
	}

	if *endTimeStr != "" {
		var err error
		opts.endTime, err = time.Parse(time.RFC3339, *endTimeStr)
		if err != nil {
			log.Fatalf("Invalid end time format: %v", err)
		}
	}

	if opts.topic == "" {
		opts.topic = cfg.Kafka.Topic
	}

	// Connect to database
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	repo := database.NewRepository(db)

	var producer *kafka.Producer
	if !opts.dryRun {
		// Create Kafka producer
		producer, err = kafka.NewProducer(&kafka.ProducerConfig{
			Brokers:  cfg.Kafka.Brokers,
			ClientID: "backfill-tool",
		})
		if err != nil {
			log.Fatalf("Failed to create Kafka producer: %v", err)
		}
		defer producer.Close()

		// Health check
		if err := producer.HealthCheck(); err != nil {
			log.Fatalf("Kafka health check failed: %v", err)
		}
	}

	backfiller := &Backfiller{
		repo:     repo,
		producer: producer,
		opts:     opts,
	}

	ctx := context.Background()

	log.Printf("Starting backfill process...")
	if opts.dryRun {
		log.Printf("DRY RUN MODE - No events will be published")
	}

	err = backfiller.Run(ctx)
	if err != nil {
		log.Fatalf("Backfill failed: %v", err)
	}

	log.Printf("Backfill completed successfully")
}

type Backfiller struct {
	repo     *database.Repository
	producer *kafka.Producer
	opts     BackfillOptions
}

func (b *Backfiller) Run(ctx context.Context) error {
	totalProcessed := 0
	totalPublished := 0

	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, event_data,
			   status, topic, partition_key, created_at, processed_at,
			   retry_count, max_retries, error_message
		FROM outbox_events 
		WHERE status = $1`

	args := []interface{}{models.StatusSent}

	if b.opts.aggregateType != "" {
		query += " AND aggregate_type = $2"
		args = append(args, b.opts.aggregateType)
	}

	query += " ORDER BY created_at ASC LIMIT $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, b.opts.batchSize)

	rows, err := b.repo.GetDB().Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		event := &models.OutboxEvent{}
		err := rows.Scan(
			&event.ID, &event.AggregateType, &event.AggregateID,
			&event.EventType, &event.EventData, &event.Status,
			&event.Topic, &event.PartitionKey, &event.CreatedAt,
			&event.ProcessedAt, &event.RetryCount, &event.MaxRetries,
			&event.ErrorMessage)
		if err != nil {
			return err
		}

		totalProcessed++

		if b.opts.dryRun {
			log.Printf("DRY RUN: Would publish event %s (type: %s)", event.ID, event.EventType)
			totalPublished++
			continue
		}

		if b.opts.topic != "" {
			event.Topic = b.opts.topic
		}

		err = b.producer.PublishEvent(event)
		if err != nil {
			log.Printf("Failed to publish event %s: %v", event.ID, err)
			continue
		}

		totalPublished++
		log.Printf("Republished event %s", event.ID)
	}

	log.Printf("Backfill summary: processed=%d, published=%d", totalProcessed, totalPublished)
	return nil
}

func printHelp() {
	fmt.Printf(`Outbox Backfill Tool

Usage: %s [options]

Options:
  -aggregate-type string  Filter by aggregate type
  -dry-run               Don't actually publish, just show what would happen
  -batch-size int        Batch size (default 100)
  -help                  Show help

`, os.Args[0])
}
