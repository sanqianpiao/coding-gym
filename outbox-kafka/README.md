# Outbox-Kafka Pattern Implementation

This project implements the outbox-kafka pattern in Go to achieve exactly-once delivery from database to Kafka.

## Architecture

The outbox pattern ensures reliable message delivery by:
1. Writing business data and outbox records in a single database transaction
2. A separate relay service polls the outbox table and publishes messages to Kafka
3. Marking messages as SENT after successful Kafka delivery

## Key Components

- **HTTP API Server** (`cmd/server`): REST API for user operations
- **Outbox Relay Service** (`cmd/relay`): Background service that processes outbox events
- **Database Migration Tool** (`cmd/migrate`): Manages database schema migrations
- **Backfill Tool** (`cmd/backfill`): Utility for replaying historical events
- **User Service**: Transactional business logic layer
- **Outbox Repository**: Database access layer with ACID guarantees
- **Kafka Producer**: Idempotent message publishing with retry logic

## Quick Start

### 1. Start Infrastructure

```bash
# Start PostgreSQL and Kafka containers
docker-compose up -d

# Wait for containers to be healthy
docker-compose ps
```

### 2. Run Database Migrations

```bash
# Apply database schema
go run ./cmd/migrate up
```

### 3. Start the Application

```bash
# Terminal 1: Start the HTTP API server
PORT=8081 go run ./cmd/server

# Terminal 2: Start the relay service
go run ./cmd/relay
```

### 4. Test the System

```bash
# Create a user (triggers outbox event)
curl -X POST http://localhost:8081/users \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com", "name": "Test User"}'

# Test crash scenario
curl -X POST http://localhost:8081/test/crash \
  -H "Content-Type: application/json" \
  -d '{"email": "crash@example.com", "name": "Crash Test", "crash_after_db": true}'

# Check health
curl http://localhost:8081/health
```

### 5. Monitor Results

```bash
# Check outbox events in database
docker exec -it outbox-postgres psql -U postgres -d outbox_db \
  -c "SELECT id, event_type, status, created_at FROM outbox_events ORDER BY created_at DESC LIMIT 5;"

# View Kafka messages in UI
open http://localhost:8080
```

## API Endpoints

### Users API
- `POST /users` - Create a new user
- `GET /users/{id}` - Get user by ID
- `PUT /users/{id}` - Update user
- `GET /health` - Health check

### Testing API
- `POST /test/crash` - Simulate crash scenarios

## Configuration

Environment variables:
```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=outbox_db

# Kafka
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=user-events

# Relay Service
RELAY_POLL_INTERVAL=5     # seconds
RELAY_BATCH_SIZE=100      # events per batch
RELAY_MAX_RETRIES=3       # retry attempts
```

## Database Schema

### Users Table
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE
);
```

### Outbox Events Table
```sql
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_data JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'NEW',
    topic VARCHAR(255) NOT NULL,
    partition_key VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE,
    processed_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    error_message TEXT
);
```

## Testing & Verification

### 1. Transactional Integrity Test
```bash
# This should create both user and outbox event atomically
curl -X POST http://localhost:8081/users \
  -H "Content-Type: application/json" \
  -d '{"email": "atomic@test.com", "name": "Atomic Test"}'
```

### 2. Crash Recovery Test
```bash
# This simulates a crash after DB write but before Kafka publish
curl -X POST http://localhost:8081/test/crash \
  -H "Content-Type: application/json" \
  -d '{"email": "crash@test.com", "name": "Crash Test", "crash_after_db": true}'

# The relay service should pick up and process the orphaned event
```

### 3. Idempotency Test
The relay service ensures each event is processed exactly once by:
- Using atomic status transitions (NEW → PROCESSING → SENT)
- Implementing retry logic with exponential backoff
- Detecting and resetting stale processing events

### 4. Run Integration Tests
```bash
# Run comprehensive test suite
go test -v ./tests/integration/...
```

## Makefile Commands

```bash
make build          # Build all binaries
make run            # Start the HTTP server
make run-relay      # Start the relay service
make test           # Run all tests
make migrate        # Run database migrations
make docker-up      # Start Docker containers
make docker-down    # Stop Docker containers
make restart        # Full restart (containers + migrations)
```

## Advanced Features

### 1. Partition Routing

The system supports multiple partition strategies:

```go
// Hash-based partitioning (default)
strategy := &kafka.HashPartitionStrategy{}

// Round-robin distribution
strategy := &kafka.RoundRobinPartitionStrategy{}

// Custom per-event-type routing
strategy := kafka.NewCustomPartitionStrategy()
strategy.SetStrategyForEventType("user.created", &kafka.UserIDPartitionStrategy{})
```

### 2. Backfill Tool

Replay historical events:

```bash
# Dry run for all user events
go run ./cmd/backfill -dry-run -aggregate-type=user

# Actual backfill with custom topic
go run ./cmd/backfill -aggregate-type=user -topic=user-events-replay
```

### 3. Monitoring

- **Database**: Query outbox_events table for processing status
- **Kafka UI**: http://localhost:8080 for message inspection
- **Application Logs**: Structured logging for all operations
- **Health Checks**: Built-in health endpoints

## Production Considerations

### 1. Database Optimization
- Use read replicas for relay service queries
- Implement proper indexing on outbox_events table
- Consider partitioning for high-volume scenarios

### 2. Kafka Configuration
- Enable idempotent producer settings
- Configure appropriate replication factor
- Monitor consumer lag and processing metrics

### 3. Observability
- Add metrics (Prometheus/StatsD)
- Implement distributed tracing
- Set up alerting for failed events

### 4. Scaling
- Run multiple relay service instances
- Use database-level coordination for event processing
- Implement circuit breakers for external dependencies

## Troubleshooting

### Common Issues

1. **Events stuck in PROCESSING state**
   - Check relay service logs
   - Verify Kafka connectivity
   - Look for stale processing timeout

2. **Duplicate messages in Kafka**
   - Verify idempotent producer configuration
   - Check for replay scenarios
   - Review partition key strategy

3. **Database connection issues**
   - Verify PostgreSQL is running
   - Check connection pool settings
   - Review migration status

## Architecture Benefits

1. **Exactly-Once Delivery**: Transactional outbox ensures no message loss
2. **Fault Tolerance**: System recovers from crashes automatically
3. **Scalability**: Relay service can be horizontally scaled
4. **Observability**: Full audit trail of all events
5. **Flexibility**: Pluggable partition strategies and custom routing

This implementation demonstrates enterprise-grade patterns for reliable event-driven architectures using the outbox pattern with Kafka.