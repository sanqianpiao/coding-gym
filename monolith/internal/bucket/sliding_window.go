package bucket

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// SlidingWindowConfig holds configuration for sliding window rate limiter
type SlidingWindowConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	WindowSize    time.Duration // Size of the sliding window
	MaxRequests   int64         // Maximum requests allowed in the window
	TTL           time.Duration // TTL for window keys
}

// DefaultSlidingWindowConfig returns a sensible default configuration
func DefaultSlidingWindowConfig() *SlidingWindowConfig {
	return &SlidingWindowConfig{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       0,
		WindowSize:    1 * time.Minute, // 1 minute window
		MaxRequests:   100,             // 100 requests per minute
		TTL:           5 * time.Minute, // 5 minute TTL
	}
}

// RedisSlidingWindow implements a sliding window rate limiter using Redis
type RedisSlidingWindow struct {
	client    *redis.Client
	config    *SlidingWindowConfig
	luaScript *redis.Script
}

// SlidingWindowResult represents the result of a sliding window check
type SlidingWindowResult struct {
	Allowed      bool    `json:"allowed"`
	CurrentCount int64   `json:"current_count"`
	WindowStart  int64   `json:"window_start"`
	WindowEnd    int64   `json:"window_end"`
	RetryAfter   float64 `json:"retry_after_seconds,omitempty"`
}

// NewRedisSlidingWindow creates a new Redis-backed sliding window rate limiter
func NewRedisSlidingWindow(config *SlidingWindowConfig) (*RedisSlidingWindow, error) {
	if config == nil {
		config = DefaultSlidingWindowConfig()
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	sw := &RedisSlidingWindow{
		client: client,
		config: config,
	}

	sw.initLuaScript()

	return sw, nil
}

// Close closes the Redis connection
func (sw *RedisSlidingWindow) Close() error {
	return sw.client.Close()
}

// keyName generates a Redis key for the given window key
func (sw *RedisSlidingWindow) keyName(key string) string {
	return fmt.Sprintf("sliding_window:%s", key)
}

// IsAllowed checks if a request is allowed within the sliding window
func (sw *RedisSlidingWindow) IsAllowed(ctx context.Context, key string) (*SlidingWindowResult, error) {
	now := time.Now()
	windowStart := now.Add(-sw.config.WindowSize)
	redisKey := sw.keyName(key)

	// Run the sliding window Lua script
	result, err := sw.luaScript.Run(ctx, sw.client, []string{redisKey},
		windowStart.UnixMilli(),
		now.UnixMilli(),
		sw.config.MaxRequests,
		sw.config.TTL.Seconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check sliding window: %w", err)
	}

	values := result.([]interface{})
	allowed := parseInt64(values[0]) == 1
	currentCount := parseInt64(values[1])
	retryAfter := parseFloat64(values[2])

	slidingResult := &SlidingWindowResult{
		Allowed:      allowed,
		CurrentCount: currentCount,
		WindowStart:  windowStart.UnixMilli(),
		WindowEnd:    now.UnixMilli(),
	}

	if !allowed && retryAfter > 0 {
		slidingResult.RetryAfter = retryAfter / 1000.0 // Convert to seconds
	}

	return slidingResult, nil
}

// GetWindowState returns the current state of the sliding window
func (sw *RedisSlidingWindow) GetWindowState(ctx context.Context, key string) (*SlidingWindowResult, error) {
	now := time.Now()
	windowStart := now.Add(-sw.config.WindowSize)
	redisKey := sw.keyName(key)

	// Get current count without adding a new request
	script := `
		local key = KEYS[1]
		local window_start = ARGV[1]
		local window_end = ARGV[2]
		
		-- Remove expired entries
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
		
		-- Count current entries
		local current_count = redis.call('ZCARD', key)
		
		return {current_count}
	`

	result, err := redis.NewScript(script).Run(ctx, sw.client, []string{redisKey},
		windowStart.UnixMilli(),
		now.UnixMilli()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get window state: %w", err)
	}

	values := result.([]interface{})
	currentCount := parseInt64(values[0])

	return &SlidingWindowResult{
		Allowed:      currentCount < sw.config.MaxRequests,
		CurrentCount: currentCount,
		WindowStart:  windowStart.UnixMilli(),
		WindowEnd:    now.UnixMilli(),
	}, nil
}

// ClearWindow clears all entries for a given key
func (sw *RedisSlidingWindow) ClearWindow(ctx context.Context, key string) error {
	redisKey := sw.keyName(key)
	return sw.client.Del(ctx, redisKey).Err()
}

// Lua script for sliding window rate limiting
const slidingWindowScript = `
local key = KEYS[1]
local window_start = ARGV[1]
local window_end = ARGV[2]
local max_requests = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

-- Remove expired entries
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

-- Count current entries
local current_count = redis.call('ZCARD', key)

local allowed = 0
local retry_after = 0

if current_count < max_requests then
    -- Add current request
    redis.call('ZADD', key, window_end, window_end)
    allowed = 1
    current_count = current_count + 1
else
    -- Calculate retry after (time until oldest entry expires)
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    if #oldest > 0 then
        local oldest_time = tonumber(oldest[2])
        local window_size = tonumber(window_end) - tonumber(window_start)
        retry_after = oldest_time + window_size - tonumber(window_end)
        if retry_after < 0 then
            retry_after = 0
        end
    end
end

-- Set TTL
redis.call('EXPIRE', key, ttl)

return {allowed, current_count, retry_after}
`

// initLuaScript initializes the Lua script
func (sw *RedisSlidingWindow) initLuaScript() {
	sw.luaScript = redis.NewScript(slidingWindowScript)
}
