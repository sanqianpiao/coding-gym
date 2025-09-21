package bucket

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkTokenBucket_TakeTokens(b *testing.B) {
	tb := createTestBucket(b, 1000000, 1000.0) // Large capacity for benchmarking
	defer tb.Close()

	ctx := context.Background()
	key := "benchmark_test"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := tb.TakeTokens(ctx, key, 1)
			if err != nil {
				b.Errorf("TakeTokens failed: %v", err)
			}
		}
	})
}

func BenchmarkTokenBucket_GetState(b *testing.B) {
	tb := createTestBucket(b, 1000000, 1000.0)
	defer tb.Close()

	ctx := context.Background()
	key := "benchmark_state_test"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := tb.GetBucketState(ctx, key)
			if err != nil {
				b.Errorf("GetBucketState failed: %v", err)
			}
		}
	})
}

func BenchmarkTokenBucket_MultipleKeys(b *testing.B) {
	tb := createTestBucket(b, 1000000, 1000.0)
	defer tb.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark_key_%d", i%100) // 100 different keys
			_, err := tb.TakeTokens(ctx, key, 1)
			if err != nil {
				b.Errorf("TakeTokens failed: %v", err)
			}
			i++
		}
	})
}
