package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"monolith/internal/bucket"
)

// RateLimitConfig holds configuration for the rate limiting middleware
type RateLimitConfig struct {
	RequestsPerMinute int           // Number of requests allowed per minute
	BurstSize         int           // Burst capacity
	RefillRate        time.Duration // How often to add tokens
}

// DefaultRateLimitConfig returns a sensible default configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerMinute: 100,         // 100 requests per minute
		BurstSize:         20,          // Allow bursts of 20 requests
		RefillRate:        time.Minute, // Refill every minute
	}
}

// RateLimitMiddleware creates a rate limiting middleware using the token bucket
type RateLimitMiddleware struct {
	tokenBucket *bucket.RedisTokenBucket
	config      *RateLimitConfig
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(tb *bucket.RedisTokenBucket, config *RateLimitConfig) *RateLimitMiddleware {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	return &RateLimitMiddleware{
		tokenBucket: tb,
		config:      config,
	}
}

// Handler returns the HTTP middleware handler function
func (rlm *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client identifier (IP address or API key)
		clientKey := rlm.getClientKey(r)

		// Use token bucket key for rate limiting
		bucketKey := fmt.Sprintf("api_rate_limit:%s", clientKey)

		// Try to consume 1 token for this request
		ctx := context.Background()
		result, err := rlm.tokenBucket.TakeTokens(ctx, bucketKey, 1)
		if err != nil {
			log.Printf("Rate limit error for %s: %v", clientKey, err)
			http.Error(w, "Rate limiting temporarily unavailable", http.StatusInternalServerError)
			return
		}

		// Fix retry_after overflow issue - cap at reasonable maximum
		retryAfter := result.RetryAfter
		if retryAfter > 3600 || retryAfter < 0 { // Cap at 1 hour max, handle negative values
			retryAfter = 60 // Default to 1 minute
		}

		// Debug logging
		log.Printf("Rate limit check for %s: allowed=%v, remaining=%.2f, retryAfter=%.2f (fixed=%.2f)",
			clientKey, result.Allowed, result.RemainingTokens, result.RetryAfter, retryAfter)

		// Check if request is allowed
		if !result.Allowed {
			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rlm.config.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", strconv.FormatFloat(retryAfter, 'f', 0, 64))

			// Log rate limit hit
			log.Printf("Rate limit exceeded for client %s (IP: %s, User-Agent: %s) - retry after %.2f seconds",
				clientKey, r.RemoteAddr, r.UserAgent(), retryAfter)

			// Return 429 Too Many Requests
			http.Error(w, fmt.Sprintf("Rate limit exceeded. Try again after %.0f seconds.",
				retryAfter), http.StatusTooManyRequests)
			return
		}

		// Add informational rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rlm.config.RequestsPerMinute))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(result.RemainingTokens, 'f', 0, 64))

		// Request is allowed, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// getClientKey extracts a unique identifier for the client
func (rlm *RateLimitMiddleware) getClientKey(r *http.Request) string {
	// Priority order for client identification:
	// 1. API Key (if provided in Authorization header)
	// 2. X-Forwarded-For (if behind proxy)
	// 3. X-Real-IP (if behind proxy)
	// 4. RemoteAddr (direct connection)

	// Check for API key in Authorization header
	if apiKey := r.Header.Get("Authorization"); apiKey != "" {
		return fmt.Sprintf("api_key:%s", apiKey)
	}

	// Check for X-API-Key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return fmt.Sprintf("api_key:%s", apiKey)
	}

	// Check for forwarded IP (when behind load balancer/proxy)
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		// Take only the first IP in case of multiple proxies
		firstIP := strings.Split(forwardedFor, ",")[0]
		return fmt.Sprintf("ip:%s", strings.TrimSpace(firstIP))
	}

	// Check for real IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return fmt.Sprintf("ip:%s", realIP)
	}

	// Fall back to remote address, strip port if present
	remoteIP := r.RemoteAddr
	if colonIdx := strings.LastIndex(remoteIP, ":"); colonIdx != -1 {
		remoteIP = remoteIP[:colonIdx]
	}
	return fmt.Sprintf("ip:%s", remoteIP)
}

// LoggingMiddleware logs all HTTP requests (compatible with mux.MiddlewareFunc)
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer that captures the status code
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(ww, r)

		// Log request details
		duration := time.Since(start)
		log.Printf("%s %s %d %v %s %s",
			r.Method,
			r.URL.Path,
			ww.statusCode,
			duration,
			r.RemoteAddr,
			r.UserAgent())
	})
}

// responseWriter is a wrapper to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// CORSMiddleware adds CORS headers (compatible with mux.MiddlewareFunc)
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware recovers from panics and returns 500 error (compatible with mux.MiddlewareFunc)
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
