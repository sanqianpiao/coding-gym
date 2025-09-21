package bucket

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config holds the configuration for the Redis token bucket
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	Capacity      int64         // Maximum tokens in bucket
	RefillRate    float64       // Tokens per second
	TTL           time.Duration // Time to live for bucket keys
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       0,
		Capacity:      100,             // 100 tokens max
		RefillRate:    10.0,            // 10 tokens per second
		TTL:           5 * time.Minute, // 5 minute TTL
	}
}

// RedisTokenBucket implements a token bucket rate limiter using Redis
type RedisTokenBucket struct {
	client     *redis.Client
	config     *Config
	luaScripts map[string]*redis.Script
}

// BucketState represents the current state of a token bucket
type BucketState struct {
	Key            string    `json:"key"`
	CurrentTokens  float64   `json:"current_tokens"`
	Capacity       int64     `json:"capacity"`
	RefillRate     float64   `json:"refill_rate"`
	LastRefillTime time.Time `json:"last_refill_time"`
	TTL            int64     `json:"ttl_seconds"`
}

// TokenResult represents the result of a token consumption attempt
type TokenResult struct {
	Allowed         bool    `json:"allowed"`
	RemainingTokens float64 `json:"remaining_tokens"`
	RetryAfter      float64 `json:"retry_after_seconds,omitempty"`
}

// NewRedisTokenBucket creates a new Redis-backed token bucket
func NewRedisTokenBucket(config *Config) (*RedisTokenBucket, error) {
	if config == nil {
		config = DefaultConfig()
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

	tb := &RedisTokenBucket{
		client:     client,
		config:     config,
		luaScripts: make(map[string]*redis.Script),
	}

	// Initialize Lua scripts
	tb.initLuaScripts()

	return tb, nil
}

// Close closes the Redis connection
func (tb *RedisTokenBucket) Close() error {
	return tb.client.Close()
}

// keyName generates a Redis key for the given bucket key
func (tb *RedisTokenBucket) keyName(key string) string {
	return fmt.Sprintf("token_bucket:%s", key)
}

// GetBucketState returns the current state of a token bucket
func (tb *RedisTokenBucket) GetBucketState(ctx context.Context, key string) (*BucketState, error) {
	redisKey := tb.keyName(key)

	// Run the get bucket state Lua script
	result, err := tb.luaScripts["get_state"].Run(ctx, tb.client, []string{redisKey},
		tb.config.Capacity, tb.config.RefillRate, tb.config.TTL.Seconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket state: %w", err)
	}

	values := result.([]interface{})
	currentTokens := parseFloat64(values[0])
	lastRefillTime := time.Unix(parseInt64(values[1]), 0)
	ttl := parseInt64(values[2])

	return &BucketState{
		Key:            key,
		CurrentTokens:  currentTokens,
		Capacity:       tb.config.Capacity,
		RefillRate:     tb.config.RefillRate,
		LastRefillTime: lastRefillTime,
		TTL:            ttl,
	}, nil
}

// TakeTokens attempts to consume the specified number of tokens
func (tb *RedisTokenBucket) TakeTokens(ctx context.Context, key string, tokens float64) (*TokenResult, error) {
	if tokens <= 0 {
		return nil, fmt.Errorf("tokens must be positive")
	}

	redisKey := tb.keyName(key)

	// Run the take tokens Lua script
	result, err := tb.luaScripts["take_tokens"].Run(ctx, tb.client, []string{redisKey},
		tokens, tb.config.Capacity, tb.config.RefillRate, tb.config.TTL.Seconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to take tokens: %w", err)
	}

	values := result.([]interface{})
	allowed := parseInt64(values[0]) == 1
	remainingTokens := parseFloat64(values[1])
	retryAfter := parseFloat64(values[2])

	tokenResult := &TokenResult{
		Allowed:         allowed,
		RemainingTokens: remainingTokens,
	}

	if !allowed && retryAfter > 0 {
		tokenResult.RetryAfter = retryAfter
	}

	return tokenResult, nil
}

// ResetBucket resets a bucket to full capacity
func (tb *RedisTokenBucket) ResetBucket(ctx context.Context, key string) error {
	redisKey := tb.keyName(key)

	_, err := tb.luaScripts["reset_bucket"].Run(ctx, tb.client, []string{redisKey},
		tb.config.Capacity, tb.config.RefillRate, tb.config.TTL.Seconds()).Result()
	if err != nil {
		return fmt.Errorf("failed to reset bucket: %w", err)
	}

	return nil
}

// Helper functions for type conversion
func parseInt64(val interface{}) int64 {
	switch v := val.(type) {
	case int64:
		return v
	case string:
		if v == "" {
			return 0
		}
		// For Redis responses, we expect string representations of numbers
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
		return 0
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func parseFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case string:
		if v == "" {
			return 0
		}
		// For Redis responses, we expect string representations of numbers
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return 0
	case int64:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}
