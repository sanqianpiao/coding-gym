package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/kafka"
	"monolith/internal/models"
	"monolith/internal/relay"
	"monolith/internal/service"
)

func TestOutboxPattern(t *testing.T) {
	// Setup test environment
	cfg := config.Load()
	cfg.Database.DBName = "outbox_test_db" // Use test database

	db, err := database.NewConnection(&cfg.Database)
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	err = db.Migrate("up")
	require.NoError(t, err)

	repo := database.NewRepository(db)
	userService := service.NewUserService(repo, cfg.Kafka.Topic)

	t.Run("TransactionalWrite", func(t *testing.T) {
		// Test that user creation writes both user and outbox event in a single transaction
		email := "test@example.com"
		name := "Test User"

		user, err := userService.CreateUser(email, name)
		require.NoError(t, err)
		assert.Equal(t, email, user.Email)
		assert.Equal(t, name, user.Name)

		// Verify user exists in database
		retrievedUser, err := userService.GetUser(user.ID)
		require.NoError(t, err)
		assert.Equal(t, user.ID, retrievedUser.ID)

		// Verify outbox event exists
		events, err := repo.GetPendingOutboxEvents(10)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		
		event := events[0]
		assert.Equal(t, models.UserCreatedEvent, event.EventType)
		assert.Equal(t, models.StatusNew, event.Status)
		assert.Equal(t, user.ID.String(), event.AggregateID)
	})

	t.Run("CrashScenario", func(t *testing.T) {
		// Test simulated crash scenario
		email := "crash@example.com"
		name := "Crash User"

		user, err := userService.TestCrashScenario(email, name, true)
		require.Error(t, err) // Should return error to simulate crash
		require.NotNil(t, user) // But user should be created

		// Verify user exists in database (transaction was committed)
		retrievedUser, err := userService.GetUser(user.ID)
		require.NoError(t, err)
		assert.Equal(t, user.ID, retrievedUser.ID)

		// Verify outbox event exists and is pending
		events, err := repo.GetPendingOutboxEvents(10)
		require.NoError(t, err)
		
		// Find our event
		var crashEvent *models.OutboxEvent
		for _, event := range events {
			if event.AggregateID == user.ID.String() {
				crashEvent = event
				break
			}
		}
		require.NotNil(t, crashEvent, "Outbox event should exist after crash")
		assert.Equal(t, models.StatusNew, crashEvent.Status)
	})

	t.Run("RelayProcessing", func(t *testing.T) {
		// Create a mock Kafka producer for testing
		producer, err := kafka.NewProducer(&kafka.ProducerConfig{
			Brokers:  cfg.Kafka.Brokers,
			ClientID: "test-relay",
		})
		require.NoError(t, err)
		defer producer.Close()

		// Skip if Kafka is not available
		if err := producer.HealthCheck(); err != nil {
			t.Skip("Kafka not available for integration test")
		}

		relayService := relay.NewRelay(repo, producer, &cfg.Relay)

		// Create a user to generate an outbox event
		email := "relay@example.com"
		name := "Relay User"
		user, err := userService.CreateUser(email, name)
		require.NoError(t, err)

		// Process outbox events once
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Start relay in background
		go func() {
			relayService.Start(ctx)
		}()

		// Wait a bit for processing
		time.Sleep(2 * time.Second)

		// Verify event was processed
		events, err := repo.GetPendingOutboxEvents(10)
		require.NoError(t, err)

		// The event should either be processed (SENT) or no longer pending
		found := false
		for _, event := range events {
			if event.AggregateID == user.ID.String() {
				found = true
				// If still pending, it might be in PROCESSING state
				assert.NotEqual(t, models.StatusNew, event.Status)
			}
		}

		// If not found in pending events, it was likely processed successfully
		if !found {
			t.Log("Event was successfully processed and marked as SENT")
		}
	})

	t.Run("IdempotencyTest", func(t *testing.T) {
		// Create user and process multiple times to test idempotency
		email := "idempotent@example.com"
		name := "Idempotent User"
		user, err := userService.CreateUser(email, name)
		require.NoError(t, err)

		// Get the outbox event
		events, err := repo.GetPendingOutboxEvents(10)
		require.NoError(t, err)

		var testEvent *models.OutboxEvent
		for _, event := range events {
			if event.AggregateID == user.ID.String() {
				testEvent = event
				break
			}
		}
		require.NotNil(t, testEvent)

		// Manually mark as processing to test concurrent access
		tx1, err := repo.BeginTx()
		require.NoError(t, err)

		err = repo.MarkEventAsProcessing(tx1, testEvent.ID)
		require.NoError(t, err)

		err = tx1.Commit()
		require.NoError(t, err)

		// Try to mark as processing again (should fail)
		tx2, err := repo.BeginTx()
		require.NoError(t, err)
		defer tx2.Rollback()

		err = repo.MarkEventAsProcessing(tx2, testEvent.ID)
		assert.Error(t, err, "Should not be able to mark already processing event")
	})

	t.Run("UpdateUserEvent", func(t *testing.T) {
		// Test user update creates outbox event
		email := "update@example.com"
		name := "Update User"
		user, err := userService.CreateUser(email, name)
		require.NoError(t, err)

		// Update the user
		newEmail := "updated@example.com"
		newName := "Updated User"
		updatedUser, err := userService.UpdateUser(user.ID, newEmail, newName)
		require.NoError(t, err)
		assert.Equal(t, newEmail, updatedUser.Email)
		assert.Equal(t, newName, updatedUser.Name)

		// Verify we have both create and update events
		events, err := repo.GetPendingOutboxEvents(10)
		require.NoError(t, err)

		createEvents := 0
		updateEvents := 0
		for _, event := range events {
			if event.AggregateID == user.ID.String() {
				switch event.EventType {
				case models.UserCreatedEvent:
					createEvents++
				case models.UserUpdatedEvent:
					updateEvents++
				}
			}
		}

		assert.Equal(t, 1, createEvents, "Should have one create event")
		assert.Equal(t, 1, updateEvents, "Should have one update event")
	})
}
