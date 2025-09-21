package bucket

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func createTestSlidingWindow(tb interface{}, windowSize time.Duration, maxRequests int64) *RedisSlidingWindow {
	config := &SlidingWindowConfig{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       2, // Use different DB for sliding window tests
		WindowSize:    windowSize,
		MaxRequests:   maxRequests,
		TTL:           5 * time.Minute,
	}

	sw, err := NewRedisSlidingWindow(config)
	if err != nil {
		switch v := tb.(type) {
		case *testing.T:
			v.Skipf("Redis not available: %v", err)
		case *testing.B:
			v.Skipf("Redis not available: %v", err)
		}
	}

	return sw
}

func TestSlidingWindow_BasicFunctionality(t *testing.T) {
	sw := createTestSlidingWindow(t, 1*time.Second, 5)
	defer sw.Close()

	ctx := context.Background()
	key := "sliding_test"

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		result, err := sw.IsAllowed(ctx, key)
		if err != nil {
			t.Fatalf("IsAllowed failed on request %d: %v", i+1, err)
		}
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
		if result.CurrentCount != int64(i+1) {
			t.Errorf("Expected count %d, got %d", i+1, result.CurrentCount)
		}
	}

	// 6th request should be denied
	result, err := sw.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("IsAllowed failed on 6th request: %v", err)
	}
	if result.Allowed {
		t.Error("Expected 6th request to be denied")
	}
	if result.CurrentCount != 5 {
		t.Errorf("Expected count 5, got %d", result.CurrentCount)
	}
}

func TestSlidingWindow_WindowExpiry(t *testing.T) {
	sw := createTestSlidingWindow(t, 200*time.Millisecond, 3) // Very short window for testing
	defer sw.Close()

	ctx := context.Background()
	key := "expiry_test"

	// Fill up the window
	for i := 0; i < 3; i++ {
		result, err := sw.IsAllowed(ctx, key)
		if err != nil {
			t.Fatalf("IsAllowed failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// Should be at capacity
	result, err := sw.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("IsAllowed failed: %v", err)
	}
	if result.Allowed {
		t.Error("Expected request to be denied when at capacity")
	}

	// Wait for window to expire
	time.Sleep(250 * time.Millisecond)

	// Should be allowed again after window expiry
	result, err = sw.IsAllowed(ctx, key)
	if err != nil {
		t.Fatalf("IsAllowed failed after window expiry: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected request to be allowed after window expiry")
	}
	if result.CurrentCount != 1 {
		t.Errorf("Expected count 1 after window expiry, got %d", result.CurrentCount)
	}
}

func TestSlidingWindow_GetWindowState(t *testing.T) {
	sw := createTestSlidingWindow(t, 1*time.Second, 10)
	defer sw.Close()

	ctx := context.Background()
	key := "state_test"

	// Add some requests
	for i := 0; i < 3; i++ {
		_, err := sw.IsAllowed(ctx, key)
		if err != nil {
			t.Fatalf("IsAllowed failed: %v", err)
		}
	}

	// Check state
	state, err := sw.GetWindowState(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowState failed: %v", err)
	}
	if state.CurrentCount != 3 {
		t.Errorf("Expected count 3, got %d", state.CurrentCount)
	}
	if !state.Allowed {
		t.Error("Expected window to allow more requests")
	}
}

func TestSlidingWindow_ClearWindow(t *testing.T) {
	sw := createTestSlidingWindow(t, 1*time.Second, 5)
	defer sw.Close()

	ctx := context.Background()
	key := "clear_test"

	// Fill up the window
	for i := 0; i < 5; i++ {
		_, err := sw.IsAllowed(ctx, key)
		if err != nil {
			t.Fatalf("IsAllowed failed: %v", err)
		}
	}

	// Verify it's full
	state, err := sw.GetWindowState(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowState failed: %v", err)
	}
	if state.CurrentCount != 5 {
		t.Errorf("Expected count 5, got %d", state.CurrentCount)
	}

	// Clear the window
	err = sw.ClearWindow(ctx, key)
	if err != nil {
		t.Fatalf("ClearWindow failed: %v", err)
	}

	// Verify it's empty
	state, err = sw.GetWindowState(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowState failed: %v", err)
	}
	if state.CurrentCount != 0 {
		t.Errorf("Expected count 0 after clear, got %d", state.CurrentCount)
	}
}

func TestSlidingWindow_ConcurrencyTest(t *testing.T) {
	sw := createTestSlidingWindow(t, 1*time.Second, 50)
	defer sw.Close()

	ctx := context.Background()
	key := "concurrency_sliding_test"

	const numGoroutines = 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failureCount := 0

	// Launch goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result, err := sw.IsAllowed(ctx, key)
			if err != nil {
				t.Errorf("IsAllowed failed in goroutine %d: %v", index, err)
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

	// Should have exactly 50 successes (max requests)
	if successCount != 50 {
		t.Errorf("Expected exactly 50 successful requests, got %d", successCount)
	}

	if failureCount != 50 {
		t.Errorf("Expected exactly 50 failed requests, got %d", failureCount)
	}

	t.Logf("Sliding window concurrency test: %d successes, %d failures", successCount, failureCount)
}

func BenchmarkSlidingWindow_IsAllowed(b *testing.B) {
	sw := createTestSlidingWindow(b, 1*time.Minute, 1000000)
	defer sw.Close()

	ctx := context.Background()
	key := "benchmark_sliding"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := sw.IsAllowed(ctx, key)
			if err != nil {
				b.Errorf("IsAllowed failed: %v", err)
			}
		}
	})
}

func BenchmarkSlidingWindow_GetState(b *testing.B) {
	sw := createTestSlidingWindow(b, 1*time.Minute, 1000000)
	defer sw.Close()

	ctx := context.Background()
	key := "benchmark_sliding_state"

	// Add some requests first
	for i := 0; i < 100; i++ {
		sw.IsAllowed(ctx, key)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := sw.GetWindowState(ctx, key)
			if err != nil {
				b.Errorf("GetWindowState failed: %v", err)
			}
		}
	})
}

// Benchmark comparison between token bucket and sliding window
func BenchmarkComparison_TokenBucketVsSlidingWindow(b *testing.B) {
	// Token bucket setup
	tb := createTestBucket(b, 1000, 1000.0)
	defer tb.Close()

	// Sliding window setup
	sw := createTestSlidingWindow(b, 1*time.Second, 1000)
	defer sw.Close()

	ctx := context.Background()

	b.Run("TokenBucket", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("tb_bench_%d", i%10)
				_, err := tb.TakeTokens(ctx, key, 1)
				if err != nil {
					b.Errorf("TakeTokens failed: %v", err)
				}
				i++
			}
		})
	})

	b.Run("SlidingWindow", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("sw_bench_%d", i%10)
				_, err := sw.IsAllowed(ctx, key)
				if err != nil {
					b.Errorf("IsAllowed failed: %v", err)
				}
				i++
			}
		})
	})
}
