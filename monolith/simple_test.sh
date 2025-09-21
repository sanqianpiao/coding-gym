#!/bin/bash

echo "Simple Rate Limit Test"
echo "====================="

# Test single request
echo "Making single request..."
response=$(curl -s -i http://localhost:8080/health)
echo "$response"
echo ""
echo "Response status:" 
echo "$response" | head -1
echo ""
echo "Rate limit headers:"
echo "$response" | grep -i "X-RateLimit"
echo "$response" | grep -i "Retry-After"
echo ""