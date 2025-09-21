# Rate Limiting Middleware

The monolith now includes a global rate limiting middleware that applies to all HTTP API calls. This middleware uses the existing Redis-backed token bucket implementation to provide distributed rate limiting.

## How It Works

The rate limiting middleware:

1. **Intercepts all HTTP requests** before they reach the API handlers
2. **Identifies clients** by IP address, API key, or forwarded headers
3. **Uses Redis token buckets** to track rate limits per client
4. **Returns HTTP 429** (Too Many Requests) when limits are exceeded
5. **Adds rate limit headers** to all responses

## Configuration

The middleware is configured in `cmd/unified-server/main.go`:

```go
// Setup middleware for global rate limiting
rateLimitConfig := &middleware.RateLimitConfig{
    RequestsPerMinute: 60,        // 60 requests per minute per client
    BurstSize:         10,        // Allow bursts of 10 requests
    RefillRate:        time.Minute, // Refill every minute
}
rateLimitMiddleware := middleware.NewRateLimitMiddleware(tb, rateLimitConfig)

// Apply global middleware stack
r.Use(middleware.RecoveryMiddleware)
r.Use(middleware.LoggingMiddleware)
r.Use(middleware.CORSMiddleware)
r.Use(rateLimitMiddleware.Handler)
```

## Client Identification

The middleware identifies clients using the following priority order:

1. **Authorization header** - API key if provided
2. **X-API-Key header** - Alternative API key header
3. **X-Forwarded-For** - IP from load balancer/proxy
4. **X-Real-IP** - Alternative forwarded IP header
5. **RemoteAddr** - Direct connection IP address

## Rate Limit Headers

All responses include rate limiting headers:

- `X-RateLimit-Limit`: Maximum requests allowed per time window
- `X-RateLimit-Remaining`: Number of tokens remaining
- `Retry-After`: Seconds to wait before retrying (on 429 responses)

## Testing Rate Limits

### Manual Testing

```bash
# Start the server
make build
PORT=8080 bin/unified-server

# Test single request (in another terminal)
curl -i http://localhost:8080/health

# Expected response includes headers:
# X-RateLimit-Limit: 60
# X-RateLimit-Remaining: 9
# Status: 200 OK

# Make rapid requests to trigger rate limiting
for i in {1..15}; do curl -i http://localhost:8080/health; done

# After 10 requests, you should see:
# Status: 429 Too Many Requests
# Retry-After: 60
```

### Automated Testing

Use the provided test script:

```bash
./test_rate_limit.sh
```

## Global Application

The rate limiting applies to **all endpoints**:

- `/health` - Health checks
- `/api/bucket/*` - Token bucket API
- `/api/users/*` - User management API
- `/api/test/*` - Test endpoints

This ensures consistent rate limiting across the entire API surface.

## Redis Integration

The middleware reuses the existing Redis token bucket infrastructure:

- **Keys**: `api_rate_limit:<client_identifier>`
- **Bucket capacity**: Configured burst size (default: 10)
- **Refill rate**: Configured refill rate (default: 1 token per minute)
- **TTL**: Automatic cleanup of unused buckets

## Customization

To modify rate limits:

1. **Edit configuration** in `main.go`:
   ```go
   rateLimitConfig := &middleware.RateLimitConfig{
       RequestsPerMinute: 100,       // Increase limit
       BurstSize:         20,        // Increase burst
       RefillRate:        time.Minute,
   }
   ```

2. **Per-endpoint limits**: Create subrouters with different middleware
3. **API key-based limits**: Different limits based on client type
4. **Dynamic limits**: Load from configuration or database

## Monitoring

The middleware logs:

- **Rate limit violations** with client information
- **Middleware errors** when Redis is unavailable
- **Request details** via logging middleware

Example log output:
```
2025/09/21 17:47:52 Rate limit exceeded for client ip:127.0.0.1 (IP: 127.0.0.1:54321, User-Agent: curl/7.64.1)
2025/09/21 17:47:52 GET /health 429 1.234ms 127.0.0.1:54321 curl/7.64.1
```

## Error Handling

The middleware handles errors gracefully:

- **Redis unavailable**: Returns 500 with fallback message
- **Token bucket errors**: Logs warning and allows request
- **Malformed headers**: Falls back to IP-based identification

## Performance Considerations

- **Minimal overhead**: Single Redis call per request
- **Connection pooling**: Reuses existing Redis connection
- **Lua scripts**: Atomic token operations in Redis
- **In-memory headers**: No additional database calls for headers

## Integration with Existing APIs

The rate limiting is transparent to existing API handlers:

- **No code changes** required in handlers
- **Automatic headers** added to all responses
- **Consistent behavior** across all endpoints
- **Compatible** with existing token bucket API

This provides a complete rate limiting solution that protects the entire monolith without requiring changes to individual API endpoints.