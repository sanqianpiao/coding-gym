#!/bin/bash

echo "Testing Rate Limiting Middleware"
echo "================================="

BASE_URL="http://localhost:8080"

echo ""
echo "Making 15 rapid requests to test rate limiting..."
echo ""

for i in {1..15}; do
    echo "Request $i:"
    response=$(curl -s -i -X GET "$BASE_URL/health" 2>&1)
    
    # Check if curl succeeded
    if echo "$response" | grep -q "curl:"; then
        echo "  ERROR: $response"
        echo "  (Server might not be running on port 8080)"
        echo ""
        break
    fi
    
    status_code=$(echo "$response" | grep -E "HTTP/[0-9.]+ [0-9]+" | awk '{print $2}')
    rate_limit_remaining=$(echo "$response" | grep -i "X-RateLimit-Remaining" | awk '{print $2}' | tr -d '\r')
    retry_after=$(echo "$response" | grep -i "Retry-After" | awk '{print $2}' | tr -d '\r')
    
    if [ -z "$status_code" ]; then
        echo "  ERROR: No status code found in response"
        echo "  Full response: $response"
    else
        echo "  Status: $status_code"
    fi
    
    if [ ! -z "$rate_limit_remaining" ]; then
        echo "  Remaining tokens: $rate_limit_remaining"
    fi
    if [ ! -z "$retry_after" ]; then
        echo "  Retry after: $retry_after seconds"
    fi
    
    echo ""
    # No delay - test burst capacity
done

echo ""
echo "Testing different endpoints to verify global rate limiting..."
echo ""

# Test different endpoints
endpoints=("/health" "/api/bucket/check?key=test" "/api/users")

for endpoint in "${endpoints[@]}"; do
    echo "Testing $endpoint:"
    response=$(curl -s -i -X GET "$BASE_URL$endpoint" 2>&1)
    status_code=$(echo "$response" | grep -E "HTTP/[0-9.]+ [0-9]+" | awk '{print $2}')
    rate_limit_remaining=$(echo "$response" | grep -i "X-RateLimit-Remaining" | awk '{print $2}' | tr -d '\r')
    
    echo "  Status: $status_code"
    if [ ! -z "$rate_limit_remaining" ]; then
        echo "  Remaining tokens: $rate_limit_remaining"
    fi
    echo ""
    sleep 1
done