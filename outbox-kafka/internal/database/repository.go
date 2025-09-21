package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"outbox-kafka/internal/models"

	"github.com/google/uuid"
)

type Repository struct {
	db *DB
}

func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// User operations
func (r *Repository) CreateUser(tx *sql.Tx, user *models.User) error {
	query := `
		INSERT INTO users (id, email, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := tx.Exec(query, user.ID, user.Email, user.Name, user.CreatedAt, user.UpdatedAt)
	return err
}

func (r *Repository) GetUserByID(id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, name, created_at, updated_at
		FROM users WHERE id = $1`

	user := &models.User{}
	err := r.db.QueryRow(query, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *Repository) UpdateUser(tx *sql.Tx, user *models.User) error {
	query := `
		UPDATE users 
		SET email = $2, name = $3, updated_at = $4
		WHERE id = $1`

	_, err := tx.Exec(query, user.ID, user.Email, user.Name, user.UpdatedAt)
	return err
}

// Outbox operations
func (r *Repository) CreateOutboxEvent(tx *sql.Tx, event *models.OutboxEvent) error {
	query := `
		INSERT INTO outbox_events (
			id, aggregate_type, aggregate_id, event_type, event_data,
			status, topic, partition_key, created_at, retry_count, max_retries
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := tx.Exec(query,
		event.ID, event.AggregateType, event.AggregateID, event.EventType,
		event.EventData, event.Status, event.Topic, event.PartitionKey,
		event.CreatedAt, event.RetryCount, event.MaxRetries)

	return err
}

func (r *Repository) GetPendingOutboxEvents(limit int) ([]*models.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, event_data,
			   status, topic, partition_key, created_at, processed_at,
			   retry_count, max_retries, error_message
		FROM outbox_events 
		WHERE status = $1 
		ORDER BY created_at ASC 
		LIMIT $2`

	rows, err := r.db.Query(query, models.StatusNew, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*models.OutboxEvent
	for rows.Next() {
		event := &models.OutboxEvent{}
		err := rows.Scan(
			&event.ID, &event.AggregateType, &event.AggregateID,
			&event.EventType, &event.EventData, &event.Status,
			&event.Topic, &event.PartitionKey, &event.CreatedAt,
			&event.ProcessedAt, &event.RetryCount, &event.MaxRetries,
			&event.ErrorMessage)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (r *Repository) MarkEventAsProcessing(tx *sql.Tx, eventID uuid.UUID) error {
	query := `
		UPDATE outbox_events 
		SET status = $1, processed_at = $2
		WHERE id = $3 AND status = $4`

	result, err := tx.Exec(query, models.StatusProcessing, time.Now(), eventID, models.StatusNew)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("event %s was already processed or not found", eventID)
	}

	return nil
}

func (r *Repository) MarkEventAsSent(tx *sql.Tx, eventID uuid.UUID) error {
	query := `
		UPDATE outbox_events 
		SET status = $1, processed_at = $2
		WHERE id = $3`

	_, err := tx.Exec(query, models.StatusSent, time.Now(), eventID)
	return err
}

func (r *Repository) MarkEventAsFailed(tx *sql.Tx, eventID uuid.UUID, errorMsg string) error {
	query := `
		UPDATE outbox_events 
		SET status = $1, retry_count = retry_count + 1, error_message = $2, processed_at = $3
		WHERE id = $4`

	_, err := tx.Exec(query, models.StatusFailed, errorMsg, time.Now(), eventID)
	return err
}

func (r *Repository) ResetStaleProcessingEvents(timeout time.Duration) error {
	query := `
		UPDATE outbox_events 
		SET status = $1, processed_at = NULL
		WHERE status = $2 AND processed_at < $3`

	cutoff := time.Now().Add(-timeout)
	_, err := r.db.Exec(query, models.StatusNew, models.StatusProcessing, cutoff)
	return err
}

// Transaction helpers
func (r *Repository) BeginTx() (*sql.Tx, error) {
	return r.db.Begin()
}

// GetDB returns the underlying database connection (for advanced queries)
func (r *Repository) GetDB() *DB {
	return r.db
}

// Helper functions for creating outbox events
func CreateUserCreatedEvent(user *models.User, topic string) (*models.OutboxEvent, error) {
	eventData := models.UserCreatedEventData{
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}

	partitionKey := user.ID.String()

	return &models.OutboxEvent{
		ID:            uuid.New(),
		AggregateType: "user",
		AggregateID:   user.ID.String(),
		EventType:     models.UserCreatedEvent,
		EventData:     data,
		Status:        models.StatusNew,
		Topic:         topic,
		PartitionKey:  &partitionKey,
		CreatedAt:     time.Now(),
		RetryCount:    0,
		MaxRetries:    3,
	}, nil
}

func CreateUserUpdatedEvent(user *models.User, topic string) (*models.OutboxEvent, error) {
	eventData := models.UserUpdatedEventData{
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.Name,
		UpdatedAt: user.UpdatedAt,
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}

	partitionKey := user.ID.String()

	return &models.OutboxEvent{
		ID:            uuid.New(),
		AggregateType: "user",
		AggregateID:   user.ID.String(),
		EventType:     models.UserUpdatedEvent,
		EventData:     data,
		Status:        models.StatusNew,
		Topic:         topic,
		PartitionKey:  &partitionKey,
		CreatedAt:     time.Now(),
		RetryCount:    0,
		MaxRetries:    3,
	}, nil
}
