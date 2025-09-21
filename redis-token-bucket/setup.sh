#!/bin/bash

# Redis Token Bucket Setup Script
# This script helps you get started with the Redis Token Bucket implementation

set -e

echo "🚀 Setting up Redis Token Bucket Rate Limiter..."

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Docker Compose is available
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose is not available. Please install Docker Compose first."
    exit 1
fi

# Function to use docker-compose or docker compose
docker_compose_cmd() {
    if command -v docker-compose &> /dev/null; then
        docker-compose "$@"
    else
        docker compose "$@"
    fi
}

# Start Redis
echo "📦 Starting Redis container..."
docker_compose_cmd up -d redis

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
sleep 3

# Test Redis connection
if docker_compose_cmd exec redis redis-cli ping | grep -q PONG; then
    echo "✅ Redis is ready!"
else
    echo "❌ Redis failed to start properly"
    exit 1
fi

# Install Go dependencies
echo "📥 Installing Go dependencies..."
if command -v go &> /dev/null; then
    go mod tidy
    echo "✅ Go dependencies installed!"
else
    echo "⚠️  Go is not installed. Please install Go to build and run the application."
fi

echo ""
echo "🎉 Setup complete!"
echo ""
echo "📋 Next steps:"
echo "1. Start the application:"
echo "   go run main.go"
echo ""
echo "2. Test the endpoints:"
echo "   # Check bucket state"
echo "   curl \"http://localhost:8080/api/check?key=user123\""
echo ""
echo "   # Consume tokens"
echo "   curl -X POST \"http://localhost:8080/api/consume?key=user123&tokens=1\""
echo ""
echo "   # Reset bucket"
echo "   curl -X POST \"http://localhost:8080/api/reset?key=user123\""
echo ""
echo "   # Health check"
echo "   curl \"http://localhost:8080/health\""
echo ""
echo "3. Run tests:"
echo "   go test -v ./..."
echo ""
echo "4. Run benchmarks:"
echo "   go test -bench=. ./..."
echo ""
echo "5. Stop services:"
echo "   docker-compose down"
echo ""
echo "📖 See README.md for more detailed information!"