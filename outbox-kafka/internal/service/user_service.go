package service

import (
	"fmt"
	"time"

	"outbox-kafka/internal/database"
	"outbox-kafka/internal/models"

	"github.com/google/uuid"
)

type UserService struct {
	repo  *database.Repository
	topic string
}

func NewUserService(repo *database.Repository, kafkaTopic string) *UserService {
	return &UserService{
		repo:  repo,
		topic: kafkaTopic,
	}
}

// CreateUser creates a user and publishes a user created event in a single transaction
func (s *UserService) CreateUser(email, name string) (*models.User, error) {
	// Start transaction
	tx, err := s.repo.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Create user entity
	user := &models.User{
		ID:        uuid.New(),
		Email:     email,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Insert user into database
	if err = s.repo.CreateUser(tx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create outbox event for user creation
	outboxEvent, err := database.CreateUserCreatedEvent(user, s.topic)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	// Insert outbox event into database
	if err = s.repo.CreateOutboxEvent(tx, outboxEvent); err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user, nil
}

// UpdateUser updates a user and publishes a user updated event in a single transaction
func (s *UserService) UpdateUser(userID uuid.UUID, email, name string) (*models.User, error) {
	// Start transaction
	tx, err := s.repo.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Get existing user (for validation)
	existingUser, err := s.repo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if existingUser == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	// Update user entity
	user := &models.User{
		ID:        userID,
		Email:     email,
		Name:      name,
		CreatedAt: existingUser.CreatedAt, // Keep original created time
		UpdatedAt: time.Now(),
	}

	// Update user in database
	if err = s.repo.UpdateUser(tx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Create outbox event for user update
	outboxEvent, err := database.CreateUserUpdatedEvent(user, s.topic)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	// Insert outbox event into database
	if err = s.repo.CreateOutboxEvent(tx, outboxEvent); err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user, nil
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(userID uuid.UUID) (*models.User, error) {
	user, err := s.repo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return user, nil
}

// TestCrashScenario simulates a crash between database write and message publishing
// This is useful for testing the outbox pattern reliability
func (s *UserService) TestCrashScenario(email, name string, crashAfterDB bool) (*models.User, error) {
	if !crashAfterDB {
		// Normal creation
		return s.CreateUser(email, name)
	}

	// Simulate crash scenario: write to DB but don't publish to Kafka
	// The relay service should pick up the event later
	tx, err := s.repo.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Always rollback to simulate crash

	user := &models.User{
		ID:        uuid.New(),
		Email:     email,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Write user
	if err = s.repo.CreateUser(tx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Write outbox event
	outboxEvent, err := database.CreateUserCreatedEvent(user, s.topic)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	if err = s.repo.CreateOutboxEvent(tx, outboxEvent); err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	// Commit the transaction first
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Now simulate the crash - return an error as if the process crashed
	// But the data is already in the database, so the relay should pick it up
	return user, fmt.Errorf("simulated crash after database write")
}
