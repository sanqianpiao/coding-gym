package bucket

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// setupTestRedis creates a test Redis client
func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       1, // Use different DB for tests
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Clear test database
	client.FlushDB(ctx)

	return client
}

// createTestBucket creates a token bucket for testing (works with both *testing.T and *testing.B)
func createTestBucket(tb interface{}, capacity int64, refillRate float64) *RedisTokenBucket {
	config := &Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       1, // Use test DB
		Capacity:      capacity,
		RefillRate:    refillRate,
		TTL:           1 * time.Minute,
	}

	bucket, err := NewRedisTokenBucket(config)
	if err != nil {
		switch v := tb.(type) {
		case *testing.T:
			v.Fatalf("Failed to create token bucket: %v", err)
		case *testing.B:
			v.Fatalf("Failed to create token bucket: %v", err)
		}
	}

	return bucket
}

func TestTokenBucket_BasicFunctionality(t *testing.T) {
	tb := createTestBucket(t, 10, 1.0) // 10 tokens, 1 token/sec
	defer tb.Close()

	ctx := context.Background()
	key := "test_user"

	// Test initial state - should have full capacity
	state, err := tb.GetBucketState(ctx, key)
	if err != nil {
		t.Fatalf("GetBucketState failed: %v", err)
	}
	if state.CurrentTokens != 10 {
		t.Errorf("Expected 10 tokens, got %.1f", state.CurrentTokens)
	}

	// Test consuming tokens
	result, err := tb.TakeTokens(ctx, key, 3)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected token consumption to be allowed")
	}
	if result.RemainingTokens != 7 {
		t.Errorf("Expected 7 remaining tokens, got %.1f", result.RemainingTokens)
	}
}

func TestTokenBucket_BoundaryCase_ZeroTokens(t *testing.T) {
	tb := createTestBucket(t, 5, 1.0) // 5 tokens, 1 token/sec
	defer tb.Close()

	ctx := context.Background()
	key := "boundary_test"

	// Consume all tokens
	for i := 0; i < 5; i++ {
		result, err := tb.TakeTokens(ctx, key, 1)
		if err != nil {
			t.Fatalf("TakeTokens failed on iteration %d: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("Expected token consumption to be allowed on iteration %d", i)
		}
	}

	// Try to consume one more token - should fail
	result, err := tb.TakeTokens(ctx, key, 1)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if result.Allowed {
		t.Error("Expected token consumption to be denied when bucket is empty")
	}
	if result.RetryAfter <= 0 {
		t.Error("Expected RetryAfter to be positive when bucket is empty")
	}
}

func TestTokenBucket_BoundaryCase_BurstConsumption(t *testing.T) {
	tb := createTestBucket(t, 10, 2.0) // 10 tokens, 2 tokens/sec
	defer tb.Close()

	ctx := context.Background()
	key := "burst_test"

	// Try to consume more tokens than capacity
	result, err := tb.TakeTokens(ctx, key, 15)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if result.Allowed {
		t.Error("Expected token consumption to be denied when requesting more than capacity")
	}

	// Consume all available tokens at once
	result, err = tb.TakeTokens(ctx, key, 10)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected full burst consumption to be allowed")
	}
	if result.RemainingTokens != 0 {
		t.Errorf("Expected 0 remaining tokens, got %.1f", result.RemainingTokens)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := createTestBucket(t, 10, 10.0) // 10 tokens, 10 tokens/sec (fast refill for testing)
	defer tb.Close()

	ctx := context.Background()
	key := "refill_test"

	// Consume all tokens
	result, err := tb.TakeTokens(ctx, key, 10)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected initial consumption to be allowed")
	}

	// Wait for some refill
	time.Sleep(500 * time.Millisecond) // Should refill ~5 tokens

	// Check if tokens were refilled
	state, err := tb.GetBucketState(ctx, key)
	if err != nil {
		t.Fatalf("GetBucketState failed: %v", err)
	}
	if state.CurrentTokens < 3 { // Allow some tolerance
		t.Errorf("Expected tokens to be refilled, got %.1f", state.CurrentTokens)
	}
}

func TestTokenBucket_ConcurrencyTest(t *testing.T) {
	tb := createTestBucket(t, 100, 10.0) // 100 tokens, 10 tokens/sec
	defer tb.Close()

	ctx := context.Background()
	key := "concurrency_test"

	const numGoroutines = 100
	const tokensPerGoroutine = 1

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failureCount := 0
	results := make([]*TokenResult, numGoroutines)

	// Launch 100 goroutines attempting to consume tokens
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result, err := tb.TakeTokens(ctx, key, tokensPerGoroutine)
			if err != nil {
				t.Errorf("TakeTokens failed in goroutine %d: %v", index, err)
				return
			}

			mu.Lock()
			results[index] = result
			if result.Allowed {
				successCount++
			} else {
				failureCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify results
	totalRequests := successCount + failureCount
	if totalRequests != numGoroutines {
		t.Errorf("Expected %d total requests, got %d", numGoroutines, totalRequests)
	}

	// Should have exactly 100 successful token consumptions (capacity)
	if successCount != 100 {
		t.Errorf("Expected exactly 100 successful token consumptions, got %d", successCount)
	}

	if failureCount != 0 {
		t.Errorf("Expected 0 failures with sufficient capacity, got %d", failureCount)
	}

	t.Logf("Concurrency test completed: %d successes, %d failures", successCount, failureCount)
}

func TestTokenBucket_ConcurrencyStress(t *testing.T) {
	tb := createTestBucket(t, 50, 5.0) // 50 tokens, 5 tokens/sec
	defer tb.Close()

	ctx := context.Background()
	key := "stress_test"

	const numGoroutines = 100
	const tokensPerGoroutine = 1

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failureCount := 0

	// Launch 100 goroutines attempting to consume tokens (should exceed capacity)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result, err := tb.TakeTokens(ctx, key, tokensPerGoroutine)
			if err != nil {
				t.Errorf("TakeTokens failed in goroutine %d: %v", index, err)
				return
			}

			mu.Lock()
			if result.Allowed {
				successCount++
			} else {
				failureCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Should have exactly 50 successful token consumptions (limited by capacity)
	if successCount != 50 {
		t.Errorf("Expected exactly 50 successful token consumptions, got %d", successCount)
	}

	if failureCount != 50 {
		t.Errorf("Expected exactly 50 failures, got %d", failureCount)
	}

	t.Logf("Stress test completed: %d successes, %d failures", successCount, failureCount)
}

func TestTokenBucket_MultipleKeys(t *testing.T) {
	tb := createTestBucket(t, 10, 2.0) // 10 tokens, 2 tokens/sec
	defer tb.Close()

	ctx := context.Background()
	keys := []string{"user1", "user2", "user3"}

	// Each key should have independent buckets
	for _, key := range keys {
		// Consume all tokens for this key
		result, err := tb.TakeTokens(ctx, key, 10)
		if err != nil {
			t.Fatalf("TakeTokens failed for key %s: %v", key, err)
		}
		if !result.Allowed {
			t.Errorf("Expected token consumption to be allowed for key %s", key)
		}

		// Try to consume one more - should fail
		result, err = tb.TakeTokens(ctx, key, 1)
		if err != nil {
			t.Fatalf("TakeTokens failed for key %s: %v", key, err)
		}
		if result.Allowed {
			t.Errorf("Expected token consumption to be denied for exhausted key %s", key)
		}
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	tb := createTestBucket(t, 10, 1.0) // 10 tokens, 1 token/sec
	defer tb.Close()

	ctx := context.Background()
	key := "reset_test"

	// Consume all tokens
	result, err := tb.TakeTokens(ctx, key, 10)
	if err != nil {
		t.Fatalf("TakeTokens failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected initial consumption to be allowed")
	}

	// Verify bucket is empty
	state, err := tb.GetBucketState(ctx, key)
	if err != nil {
		t.Fatalf("GetBucketState failed: %v", err)
	}
	if state.CurrentTokens != 0 {
		t.Errorf("Expected 0 tokens after consumption, got %.1f", state.CurrentTokens)
	}

	// Reset bucket
	err = tb.ResetBucket(ctx, key)
	if err != nil {
		t.Fatalf("ResetBucket failed: %v", err)
	}

	// Verify bucket is full again
	state, err = tb.GetBucketState(ctx, key)
	if err != nil {
		t.Fatalf("GetBucketState failed: %v", err)
	}
	if state.CurrentTokens != 10 {
		t.Errorf("Expected 10 tokens after reset, got %.1f", state.CurrentTokens)
	}
}
