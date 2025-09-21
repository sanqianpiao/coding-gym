package kafka

import (
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"
	"outbox-kafka/internal/models"
)

type Producer struct {
	producer sarama.SyncProducer
	config   *sarama.Config
}

type ProducerConfig struct {
	Brokers  []string
	ClientID string
}

func NewProducer(cfg *ProducerConfig) (*Producer, error) {
	config := sarama.NewConfig()
	
	// Producer settings for reliability
	config.Producer.RequiredAcks = sarama.WaitForAll // Wait for all replicas
	config.Producer.Retry.Max = 5                    // Retry up to 5 times
	config.Producer.Retry.Backoff = 100 * time.Millisecond
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	
	// Enable idempotent producer
	config.Producer.Idempotent = true
	config.Net.MaxOpenRequests = 1
	
	// Compression
	config.Producer.Compression = sarama.CompressionSnappy
	
	// Timeout settings
	config.Net.DialTimeout = 30 * time.Second
	config.Net.ReadTimeout = 30 * time.Second
	config.Net.WriteTimeout = 30 * time.Second
	
	// Client ID for debugging
	if cfg.ClientID != "" {
		config.ClientID = cfg.ClientID
	} else {
		config.ClientID = "outbox-relay-producer"
	}
	
	producer, err := sarama.NewSyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}
	
	return &Producer{
		producer: producer,
		config:   config,
	}, nil
}

// PublishEvent publishes an outbox event to Kafka
func (p *Producer) PublishEvent(event *models.OutboxEvent) error {
	// Create Kafka message
	msg := &sarama.ProducerMessage{
		Topic: event.Topic,
		Value: sarama.ByteEncoder(event.EventData),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event_type"),
				Value: []byte(event.EventType),
			},
			{
				Key:   []byte("aggregate_type"),
				Value: []byte(event.AggregateType),
			},
			{
				Key:   []byte("aggregate_id"),
				Value: []byte(event.AggregateID),
			},
			{
				Key:   []byte("event_id"),
				Value: []byte(event.ID.String()),
			},
			{
				Key:   []byte("created_at"),
				Value: []byte(event.CreatedAt.Format(time.RFC3339)),
			},
		},
		Timestamp: event.CreatedAt,
	}
	
	// Set partition key if specified (for partition routing)
	if event.PartitionKey != nil && *event.PartitionKey != "" {
		msg.Key = sarama.StringEncoder(*event.PartitionKey)
	}
	
	// Send message synchronously
	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to publish message to Kafka: %w", err)
	}
	
	log.Printf("Successfully published event %s to topic %s (partition: %d, offset: %d)",
		event.ID, event.Topic, partition, offset)
	
	return nil
}

// PublishEvents publishes multiple events in batch (for efficiency)
func (p *Producer) PublishEvents(events []*models.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	
	// Create messages
	messages := make([]*sarama.ProducerMessage, len(events))
	for i, event := range events {
		messages[i] = &sarama.ProducerMessage{
			Topic: event.Topic,
			Value: sarama.ByteEncoder(event.EventData),
			Headers: []sarama.RecordHeader{
				{Key: []byte("event_type"), Value: []byte(event.EventType)},
				{Key: []byte("aggregate_type"), Value: []byte(event.AggregateType)},
				{Key: []byte("aggregate_id"), Value: []byte(event.AggregateID)},
				{Key: []byte("event_id"), Value: []byte(event.ID.String())},
				{Key: []byte("created_at"), Value: []byte(event.CreatedAt.Format(time.RFC3339))},
			},
			Timestamp: event.CreatedAt,
		}
		
		if event.PartitionKey != nil && *event.PartitionKey != "" {
			messages[i].Key = sarama.StringEncoder(*event.PartitionKey)
		}
	}
	
	// Send all messages
	for i, msg := range messages {
		partition, offset, err := p.producer.SendMessage(msg)
		if err != nil {
			return fmt.Errorf("failed to publish event %s: %w", events[i].ID, err)
		}
		
		log.Printf("Published event %s to topic %s (partition: %d, offset: %d)",
			events[i].ID, events[i].Topic, partition, offset)
	}
	
	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	if p.producer != nil {
		return p.producer.Close()
	}
	return nil
}

// HealthCheck verifies the producer can connect to Kafka
func (p *Producer) HealthCheck() error {
	// Create a test client to check broker connectivity
	config := sarama.NewConfig()
	config.Net.DialTimeout = 5 * time.Second
	
	client, err := sarama.NewClient([]string{"localhost:9092"}, config)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka brokers: %w", err)
	}
	defer client.Close()
	
	// Check if we can get broker list
	brokers := client.Brokers()
	if len(brokers) == 0 {
		return fmt.Errorf("no Kafka brokers available")
	}
	
	log.Printf("Kafka health check passed. Connected to %d brokers", len(brokers))
	return nil
}
