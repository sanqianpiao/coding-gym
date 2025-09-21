# Monolith Service

A unified monolith application that combines two independent microservices:
- **Redis Token Bucket**: Rate limiting service using Redis-backed token bucket algorithm
- **Outbox-Kafka**: User management service with exactly-once delivery to Kafka using the outbox pattern

## 🏗️ Architecture

The monolith provides a single HTTP API that exposes both services:

### Token Bucket Service (`/api/bucket/*`)
- Redis-backed rate limiting using token bucket algorithm
- Sliding window implementation for precise rate control
- Bulk token consumption support
- Configurable bucket parameters

### User Management Service (`/api/users/*`) 
- PostgreSQL-backed user CRUD operations
- Outbox pattern for exactly-once event delivery to Kafka
- Transactional guarantees between domain and event storage
- Background relay service for reliable message publishing

### Shared Infrastructure
- **PostgreSQL**: User data and outbox events
- **Redis**: Token bucket state storage  
- **Kafka**: Event streaming platform
- **Unified HTTP API**: Single entry point for both services

## 🚀 Quick Start

### 1. Start Infrastructure
```bash
# Start all backing services (PostgreSQL, Redis, Kafka)
make docker-up

# Run database migrations
make migrate
```

### 2. Start the Application
```bash
# Terminal 1: Start unified HTTP server
make unified-server

# Terminal 2: Start outbox relay service  
make relay
```

### 3. Test Both APIs
```bash
# Test Token Bucket API
curl -X GET "http://localhost:8080/api/bucket/check?key=user123"
curl -X POST "http://localhost:8080/api/bucket/consume?key=user123&tokens=1"

# Test User Management API
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com", "name": "Test User"}'
```

## 📡 API Endpoints

### Token Bucket API (`/api/bucket`)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/bucket/check?key={key}` | Check available tokens |
| POST | `/api/bucket/consume?key={key}&tokens={n}` | Consume tokens |
| POST | `/api/bucket/reset` | Reset bucket state |
| POST | `/api/bucket/bulk-consume` | Bulk consume tokens |

### User Management API (`/api/users`)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/users` | Create new user |
| GET | `/api/users/{id}` | Get user by ID |
| PUT | `/api/users/{id}` | Update user |

### Testing & Health APIs
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/test/crash` | Simulate crash scenarios |
| GET | `/health` | Overall health check |
| GET | `/health/bucket` | Token bucket health |
| GET | `/health/users` | User service health |

## 🛠️ Available Commands

### Development
```bash
make setup              # Complete setup (infrastructure + migrations)
make run               # Full application stack
make unified-server    # Start unified HTTP server
make relay            # Start outbox relay service
```

### Building & Testing
```bash
make build            # Build all binaries
make test             # Run unit tests
make integration-test # Run integration tests
make test-all         # Run all tests
```

### Infrastructure Management
```bash
make docker-up        # Start all containers
make docker-down      # Stop all containers
make restart          # Full restart
make status           # Check service status
```

### Database Operations
```bash
make migrate          # Apply migrations
make migrate-down     # Rollback migrations
make db-shell         # PostgreSQL shell
make redis-shell      # Redis shell
make check-outbox     # View outbox events
```

### Testing Helpers
```bash
make test-token-bucket # Test token bucket API
make test-user-api     # Test user management API
make test-crash        # Test crash scenarios
```

## 🔄 Data Flow

### Token Bucket Flow
1. Client requests tokens via `/api/bucket/consume`
2. Unified server routes to token bucket handler
3. Handler executes Redis Lua scripts for atomic operations
4. Returns token availability and updated state

### User Management Flow (Outbox Pattern)
1. Client creates/updates user via `/api/users/*`
2. Unified server routes to user service
3. Service executes database transaction:
   - Insert/update user record
   - Insert outbox event record
4. Background relay service:
   - Polls outbox for NEW events
   - Publishes to Kafka
   - Marks events as SENT
5. Guarantees exactly-once delivery

## 🏗️ Project Structure

```
monolith/
├── cmd/
│   ├── unified-server/     # Main HTTP server (both APIs)
│   ├── relay/             # Outbox relay service
│   ├── migrate/           # Database migration tool
│   └── backfill/          # Event replay utility
├── internal/
│   ├── bucket/            # Token bucket implementation
│   ├── handler/           # Token bucket HTTP handlers
│   ├── config/            # Configuration management
│   ├── database/          # Database layer
│   ├── kafka/             # Kafka producer & partitioning
│   ├── models/            # Data models
│   ├── relay/             # Outbox relay logic
│   └── service/           # User business logic
├── migrations/            # Database schema migrations
├── tests/
│   └── integration/       # Integration tests
├── docker-compose.yml     # All infrastructure services
├── Makefile              # Comprehensive build/run commands
└── README.md             # This file
```

## ⚙️ Configuration

### Environment Variables
```bash
# Database (PostgreSQL)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres  
DB_NAME=outbox_db

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Kafka
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=user-events

# Server
PORT=8080

# Relay Service
RELAY_POLL_INTERVAL=5     # seconds
RELAY_BATCH_SIZE=100      # events per batch
RELAY_MAX_RETRIES=3       # retry attempts
```

## 🧪 Testing Scenarios

### 1. Rate Limiting Test
```bash
# Check available tokens
curl -X GET "http://localhost:8080/api/bucket/check?key=test-user"

# Consume tokens rapidly
for i in {1..10}; do
  curl -X POST "http://localhost:8080/api/bucket/consume?key=test-user&tokens=1"
done
```

### 2. Transactional Integrity Test
```bash
# Create user (should create both user and outbox event)
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email": "atomic@test.com", "name": "Atomic Test"}'

# Verify in database
make check-outbox
```

### 3. Crash Recovery Test
```bash
# Simulate crash after DB write but before Kafka publish
curl -X POST http://localhost:8080/api/test/crash \
  -H "Content-Type: application/json" \
  -d '{"email": "crash@test.com", "name": "Crash Test", "crash_after_db": true}'

# Relay service should pick up orphaned event
make logs-relay
```

## 📊 Monitoring

### Service Health
- **Unified Server**: `GET /health`
- **Individual Services**: `GET /health/bucket` and `GET /health/users`
- **Infrastructure**: `make status`

### Data Monitoring  
- **Kafka UI**: http://localhost:8081
- **Database Events**: `make check-outbox`
- **Redis State**: `make redis-shell` → `KEYS *`

### Logs
```bash
make docker-logs      # All container logs
make logs-db         # PostgreSQL logs
make logs-redis      # Redis logs  
make logs-kafka      # Kafka logs
```

## 🚀 Production Considerations

### Scaling
- **Horizontal**: Run multiple unified server instances behind load balancer
- **Vertical**: Adjust container resources based on load patterns
- **Database**: Use read replicas for relay service queries
- **Redis**: Cluster mode for high availability

### Security
- Add authentication/authorization middleware
- Use TLS for all external connections
- Secure Redis and PostgreSQL with passwords
- Implement API rate limiting at gateway level

### Observability
- Add metrics collection (Prometheus)
- Implement distributed tracing
- Set up alerting for failed outbox events
- Monitor token bucket exhaustion rates

## 🔄 Migration from Microservices

This monolith was created by merging two independent services:

### Original Services
- `outbox-kafka/`: User management with exactly-once Kafka delivery
- `redis-token-bucket/`: Rate limiting service

### Key Changes Made
1. **Unified Go Module**: Combined dependencies in single `go.mod`
2. **Merged Internal Packages**: No naming conflicts, clean separation
3. **Unified HTTP Server**: Single entry point with API prefixes
4. **Combined Infrastructure**: Single `docker-compose.yml` with all services
5. **Integrated Configuration**: Shared config loading with environment variables
6. **Comprehensive Makefile**: All operations in single command interface

### Benefits of Monolith
- **Simplified Deployment**: Single binary and container
- **Shared Infrastructure**: Reduced resource overhead
- **Unified API**: Single endpoint for clients
- **Easier Development**: Single codebase to build and test
- **Transactional Integrity**: Can share database transactions if needed

### Microservice Benefits Retained
- **Service Boundaries**: Clean separation via API prefixes
- **Independent Scaling**: Can still scale components independently
- **Technology Diversity**: Each service uses optimal tech stack
- **Future Migration**: Easy to extract back to microservices

## 🎯 Next Steps

- [ ] Add API authentication and authorization
- [ ] Implement circuit breakers for external dependencies  
- [ ] Add comprehensive metrics and monitoring
- [ ] Create Kubernetes deployment manifests
- [ ] Implement graceful degradation for service failures
- [ ] Add API versioning strategy
- [ ] Performance testing and optimization