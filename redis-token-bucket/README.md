# Redis Token Bucket Rate Limiter

A high-performance, Redis-backed token bucket rate limiter implementation in Go with atomic operations and comprehensive testing.

## Features

- **Per-key rate limiting**: Support for user-based or IP-based rate limiting
- **Atomic operations**: Uses Redis Lua scripts for thread-safe token bucket operations
- **TTL management**: Automatic cleanup of expired buckets
- **HTTP API**: Simple REST endpoints for demonstration
- **Comprehensive testing**: Includes concurrency tests and boundary case testing
- **Docker support**: Easy setup with Docker Compose

## Architecture

- **Token Bucket**: Classic rate limiting algorithm with configurable capacity and refill rate
- **Redis Backend**: All bucket state stored in Redis for scalability and persistence
- **Lua Scripts**: Atomic operations for token consumption and bucket refilling
- **HTTP Demo**: RESTful API demonstrating rate limiting in action

## Quick Start

1. Start Redis:
```bash
docker-compose up -d redis
```

2. Run the application:
```bash
go run main.go
```

3. Test the endpoints:
```bash
# Check current bucket state
curl "http://localhost:8081/api/check?key=user123"

# Consume tokens
curl -X POST "http://localhost:8081/api/consume?key=user123&tokens=1"
```

## API Endpoints

- `GET /api/check?key=<key>` - Check current bucket state
- `POST /api/consume?key=<key>&tokens=<n>` - Attempt to consume N tokens
- `GET /health` - Health check endpoint

## Testing

Run all tests including concurrency tests:
```bash
go test -v ./...
```

Run benchmarks:
```bash
go test -bench=. ./...
```

## Configuration

The token bucket can be configured with:
- `Capacity`: Maximum number of tokens in the bucket
- `RefillRate`: Tokens per second refill rate
- `TTL`: Time-to-live for bucket keys in Redis