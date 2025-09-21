# Redis Token Bucket Rate Limiter Makefile

.PHONY: setup build run test bench clean docker-up docker-down help

# Default target
help:
	@echo "Redis Token Bucket Rate Limiter"
	@echo ""
	@echo "Available targets:"
	@echo "  setup        - Run setup script to initialize the project"
	@echo "  build        - Build the application"
	@echo "  run          - Start the application"
	@echo "  test         - Run all tests"
	@echo "  test-unit    - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  bench        - Run benchmarks"
	@echo "  bench-compare - Run comparison benchmarks"
	@echo "  docker-up    - Start Redis with Docker Compose"
	@echo "  docker-down  - Stop Docker services"
	@echo "  docker-logs  - Show Docker logs"
	@echo "  clean        - Clean build artifacts and test cache"
	@echo "  deps         - Download and tidy dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter (requires golangci-lint)"
	@echo "  demo         - Run a quick demo of the rate limiter"

# Setup the project
setup:
	@./setup.sh

# Build the application
build:
	@echo "🔨 Building application..."
	@go build -o bin/token-bucket-server main.go
	@echo "✅ Build complete! Binary: bin/token-bucket-server"

# Run the application
run:
	@echo "🚀 Starting Redis Token Bucket server..."
	@go run main.go

# Download and tidy dependencies
deps:
	@echo "📥 Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies updated!"

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted!"

# Run all tests
test:
	@echo "🧪 Running all tests..."
	@go test -v ./...

# Run unit tests only
test-unit:
	@echo "🧪 Running unit tests..."
	@go test -v ./internal/bucket -run "Test.*" -short

# Run integration tests
test-integration:
	@echo "🧪 Running integration tests..."
	@go test -v ./internal/handler

# Run benchmarks
bench:
	@echo "⚡ Running benchmarks..."
	@go test -bench=. -benchmem ./...

# Run comparison benchmarks
bench-compare:
	@echo "⚡ Running comparison benchmarks..."
	@go test -bench=BenchmarkComparison -benchmem ./internal/bucket

# Start Docker services
docker-up:
	@echo "🐳 Starting Docker services..."
	@docker-compose up -d
	@echo "✅ Docker services started!"

# Stop Docker services
docker-down:
	@echo "🐳 Stopping Docker services..."
	@docker-compose down
	@echo "✅ Docker services stopped!"

# Show Docker logs
docker-logs:
	@docker-compose logs -f

# Clean build artifacts and test cache
clean:
	@echo "🧹 Cleaning up..."
	@go clean -testcache
	@rm -rf bin/
	@echo "✅ Cleanup complete!"

# Run linter (requires golangci-lint)
lint:
	@echo "🔍 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Quick demo of the rate limiter
demo: docker-up
	@echo "🎯 Running rate limiter demo..."
	@echo ""
	@echo "Starting server in background..."
	@go run main.go &
	@SERVER_PID=$$!; \
	sleep 3; \
	echo "Testing rate limiter endpoints..."; \
	echo ""; \
	echo "1. Health check:"; \
	curl -s "http://localhost:8080/health" | python3 -m json.tool || curl -s "http://localhost:8080/health"; \
	echo ""; \
	echo ""; \
	echo "2. Check initial bucket state:"; \
	curl -s "http://localhost:8080/api/check?key=demo_user" | python3 -m json.tool || curl -s "http://localhost:8080/api/check?key=demo_user"; \
	echo ""; \
	echo ""; \
	echo "3. Consume 3 tokens:"; \
	curl -s -X POST "http://localhost:8080/api/consume?key=demo_user&tokens=3" | python3 -m json.tool || curl -s -X POST "http://localhost:8080/api/consume?key=demo_user&tokens=3"; \
	echo ""; \
	echo ""; \
	echo "4. Check bucket state after consumption:"; \
	curl -s "http://localhost:8080/api/check?key=demo_user" | python3 -m json.tool || curl -s "http://localhost:8080/api/check?key=demo_user"; \
	echo ""; \
	echo ""; \
	echo "5. Try to consume more tokens than available:"; \
	curl -s -X POST "http://localhost:8080/api/consume?key=demo_user&tokens=15" | python3 -m json.tool || curl -s -X POST "http://localhost:8080/api/consume?key=demo_user&tokens=15"; \
	echo ""; \
	echo ""; \
	echo "6. Bulk consume test:"; \
	curl -s -X POST "http://localhost:8080/api/bulk-consume?key=demo_user" | python3 -m json.tool || curl -s -X POST "http://localhost:8080/api/bulk-consume?key=demo_user"; \
	echo ""; \
	echo ""; \
	echo "🎉 Demo complete!"; \
	kill $$SERVER_PID 2>/dev/null || true