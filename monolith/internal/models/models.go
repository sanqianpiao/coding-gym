package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents the domain entity
type User struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// OutboxEvent represents an event to be published to Kafka
type OutboxEvent struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	AggregateType string     `json:"aggregate_type" db:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id" db:"aggregate_id"`
	EventType     string     `json:"event_type" db:"event_type"`
	EventData     []byte     `json:"event_data" db:"event_data"`
	Status        string     `json:"status" db:"status"`
	Topic         string     `json:"topic" db:"topic"`
	PartitionKey  *string    `json:"partition_key" db:"partition_key"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	ProcessedAt   *time.Time `json:"processed_at" db:"processed_at"`
	RetryCount    int        `json:"retry_count" db:"retry_count"`
	MaxRetries    int        `json:"max_retries" db:"max_retries"`
	ErrorMessage  *string    `json:"error_message" db:"error_message"`
}

// OutboxEventStatus constants
const (
	StatusNew        = "NEW"
	StatusProcessing = "PROCESSING"
	StatusSent       = "SENT"
	StatusFailed     = "FAILED"
)

// EventTypes
const (
	UserCreatedEvent = "user.created"
	UserUpdatedEvent = "user.updated"
	UserDeletedEvent = "user.deleted"
)

// UserCreatedEventData represents the payload for user created events
type UserCreatedEventData struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// UserUpdatedEventData represents the payload for user updated events
type UserUpdatedEventData struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}
