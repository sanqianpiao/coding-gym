# Monolith Makefile - Combines Token Bucket and Outbox-Kafka services

.PHONY: help build test clean docker-up docker-down migrate unified-server relay backfill integration-test

# Variables
UNIFIED_SERVER_CMD = cmd/unified-server
RELAY_CMD = cmd/relay
MIGRATE_CMD = cmd/migrate
BACKFILL_CMD = cmd/backfill

BUILD_DIR = ./bin

help: ## Show this help message
	@echo "Monolith Service Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build all binaries
	@echo "Building all monolith services..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/unified-server ./$(UNIFIED_SERVER_CMD)
	go build -o $(BUILD_DIR)/relay ./$(RELAY_CMD)
	go build -o $(BUILD_DIR)/migrate ./$(MIGRATE_CMD)
	go build -o $(BUILD_DIR)/backfill ./$(BACKFILL_CMD)
	@echo "All binaries built successfully!"

unified-server: ## Run the unified HTTP server (Token Bucket + User Management)
	@echo "Starting unified server..."
	PORT=8080 go run ./$(UNIFIED_SERVER_CMD)

relay: ## Run the outbox relay service
	@echo "Starting relay service..."
	go run ./$(RELAY_CMD)

migrate: ## Run database migrations
	@echo "Running database migrations..."
	go run ./$(MIGRATE_CMD) up

migrate-down: ## Rollback database migrations
	@echo "Rolling back database migrations..."
	go run ./$(MIGRATE_CMD) down

backfill: ## Run backfill tool (dry-run by default)
	@echo "Running backfill tool (dry-run)..."
	go run ./$(BACKFILL_CMD) -dry-run -aggregate-type=user

backfill-execute: ## Execute actual backfill
	@echo "Executing backfill..."
	go run ./$(BACKFILL_CMD) -aggregate-type=user

test: ## Run unit tests
	@echo "Running unit tests..."
	go test -v ./internal/...

integration-test: ## Run integration tests
	@echo "Running integration tests..."
	go test -v ./tests/integration/...

test-all: test integration-test ## Run all tests

# Docker and Infrastructure Commands

docker-up: ## Start all infrastructure services (PostgreSQL, Redis, Kafka)
	@echo "Starting infrastructure services..."
	docker-compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 10

docker-down: ## Stop all infrastructure services
	@echo "Stopping infrastructure services..."
	docker-compose down

docker-logs: ## View logs from all containers
	docker-compose logs -f

# Complete setup and run

setup: docker-up migrate ## Complete setup (start infrastructure + run migrations)
	@echo "Setup complete! Infrastructure is running and database is migrated."
	@echo ""
	@echo "Available endpoints:"
	@echo "  Unified Server:  http://localhost:8080"
	@echo "  Kafka UI:        http://localhost:8080 (if not running unified server)"
	@echo ""
	@echo "To start the application:"
	@echo "  make unified-server    # Start unified HTTP server"
	@echo "  make relay            # Start relay service (in another terminal)"

run: ## Full run: setup + start unified server + relay (requires 2 terminals)
	@echo "Starting full application stack..."
	@echo "This will start the unified server. Run 'make relay' in another terminal."
	$(MAKE) setup
	$(MAKE) unified-server

restart: docker-down docker-up migrate ## Full restart (stop, start, migrate)

clean: ## Clean up build artifacts and stop containers
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)
	docker-compose down -v
	docker system prune -f

# Development helpers

dev-unified: ## Development mode: start unified server with auto-restart
	@echo "Starting unified server in development mode..."
	PORT=8080 air -c .air.toml -- ./$(UNIFIED_SERVER_CMD)

dev-relay: ## Development mode: start relay with auto-restart  
	@echo "Starting relay service in development mode..."
	air -c .air.toml -- ./$(RELAY_CMD)

# Testing scenarios

test-token-bucket: ## Test token bucket API
	@echo "Testing token bucket API..."
	curl -X GET "http://localhost:8080/api/bucket/check?key=test-key"
	@echo ""
	curl -X POST "http://localhost:8080/api/bucket/consume?key=test-key&tokens=1"
	@echo ""

test-user-api: ## Test user management API
	@echo "Testing user management API..."
	curl -X POST http://localhost:8080/api/users \
		-H "Content-Type: application/json" \
		-d '{"email": "test@example.com", "name": "Test User"}'
	@echo ""

test-crash: ## Test crash scenario
	@echo "Testing crash scenario..."
	curl -X POST http://localhost:8080/api/test/crash \
		-H "Content-Type: application/json" \
		-d '{"email": "crash@test.com", "name": "Crash Test", "crash_after_db": true}'
	@echo ""

# Monitoring and debugging

logs-db: ## View PostgreSQL logs
	docker logs -f monolith-postgres

logs-redis: ## View Redis logs
	docker logs -f monolith-redis

logs-kafka: ## View Kafka logs
	docker logs -f monolith-kafka

db-shell: ## Connect to PostgreSQL shell
	docker exec -it monolith-postgres psql -U postgres -d outbox_db

redis-shell: ## Connect to Redis shell
	docker exec -it monolith-redis redis-cli

# Status checks

status: ## Check status of all services
	@echo "Infrastructure Status:"
	@docker-compose ps
	@echo ""
	@echo "Health Checks:"
	@curl -s http://localhost:8080/health 2>/dev/null | jq . || echo "Unified server not running"
	@echo ""

check-outbox: ## Check outbox events in database
	@echo "Recent outbox events:"
	docker exec -it monolith-postgres psql -U postgres -d outbox_db \
		-c "SELECT id, event_type, status, created_at FROM outbox_events ORDER BY created_at DESC LIMIT 5;"