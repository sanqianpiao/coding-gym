.PHONY: build run test clean migrate migrate-down docker-up docker-down

# Build the application
build:
	go build -o bin/outbox-kafka ./cmd/server

# Run the application
run:
	go run ./cmd/server

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run database migrations
migrate:
	go run ./cmd/migrate up

# Rollback database migrations
migrate-down:
	go run ./cmd/migrate down

# Start infrastructure containers
docker-up:
	docker-compose up -d

# Stop infrastructure containers
docker-down:
	docker-compose down

# Rebuild and restart everything
restart: docker-down docker-up migrate

# Run the relay service
run-relay:
	go run ./cmd/relay

# Run integration tests
test-integration:
	docker-compose up -d
	sleep 10
	go test -v ./tests/integration/...
	docker-compose down