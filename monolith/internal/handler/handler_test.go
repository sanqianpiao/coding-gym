package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"monolith/internal/bucket"
	"strings"
	"testing"
	"time"
)

func createTestHandler(t *testing.T) *Handler {
	config := &bucket.Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       1, // Use test DB
		Capacity:      10,
		RefillRate:    2.0,
		TTL:           1 * time.Minute,
	}

	tb, err := bucket.NewRedisTokenBucket(config)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Clear test data
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Reset any test buckets
	tb.ResetBucket(ctx, "test_user")

	return NewHandler(tb)
}

func TestHandler_Health(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status to be 'healthy', got %v", response["status"])
	}
}

func TestHandler_CheckRate(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	// Test with valid key
	req := httptest.NewRequest("GET", "/api/check?key=test_user", nil)
	w := httptest.NewRecorder()

	h.CheckRate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response CheckRateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}
	if response.CurrentTokens != 10 {
		t.Errorf("Expected 10 tokens, got %.1f", response.CurrentTokens)
	}

	// Test without key parameter
	req = httptest.NewRequest("GET", "/api/check", nil)
	w = httptest.NewRecorder()

	h.CheckRate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandler_ConsumeTokens(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	// Test successful consumption
	req := httptest.NewRequest("POST", "/api/consume?key=test_user&tokens=3", nil)
	w := httptest.NewRecorder()

	h.ConsumeTokens(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response ConsumeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}
	if !response.Allowed {
		t.Error("Expected consumption to be allowed")
	}
	if response.RemainingTokens != 7 {
		t.Errorf("Expected 7 remaining tokens, got %.1f", response.RemainingTokens)
	}

	// Test consumption that exceeds capacity
	req = httptest.NewRequest("POST", "/api/consume?key=test_user&tokens=15", nil)
	w = httptest.NewRecorder()

	h.ConsumeTokens(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}

	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Success {
		t.Error("Expected success to be false")
	}
	if response.Allowed {
		t.Error("Expected consumption to be denied")
	}
}

func TestHandler_ResetBucket(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	// First consume some tokens
	req := httptest.NewRequest("POST", "/api/consume?key=reset_test&tokens=8", nil)
	w := httptest.NewRecorder()
	h.ConsumeTokens(w, req)

	// Reset the bucket
	req = httptest.NewRequest("POST", "/api/reset?key=reset_test", nil)
	w = httptest.NewRecorder()

	h.ResetBucket(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	// Verify bucket is reset by checking state
	req = httptest.NewRequest("GET", "/api/check?key=reset_test", nil)
	w = httptest.NewRecorder()
	h.CheckRate(w, req)

	var checkResponse CheckRateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &checkResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if checkResponse.CurrentTokens != 10 {
		t.Errorf("Expected bucket to be reset to 10 tokens, got %.1f", checkResponse.CurrentTokens)
	}
}

func TestHandler_BulkConsume(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	req := httptest.NewRequest("POST", "/api/bulk-consume?key=bulk_test", nil)
	w := httptest.NewRecorder()

	h.BulkConsume(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	// Check that results array exists and has correct length
	results, ok := response["results"].([]interface{})
	if !ok {
		t.Fatal("Expected results to be an array")
	}

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}

func TestHandler_InvalidRequests(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	testCases := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
	}{
		{"Missing key in check", "GET", "/api/check", http.StatusBadRequest},
		{"Missing key in consume", "POST", "/api/consume", http.StatusBadRequest},
		{"Invalid tokens parameter", "POST", "/api/consume?key=test&tokens=invalid", http.StatusBadRequest},
		{"Negative tokens", "POST", "/api/consume?key=test&tokens=-1", http.StatusBadRequest},
		{"Zero tokens", "POST", "/api/consume?key=test&tokens=0", http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			w := httptest.NewRecorder()

			switch {
			case strings.Contains(tc.url, "/api/check"):
				h.CheckRate(w, req)
			case strings.Contains(tc.url, "/api/consume"):
				h.ConsumeTokens(w, req)
			}

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandler_ConcurrentRequests(t *testing.T) {
	h := createTestHandler(t)
	defer h.bucket.Close()

	const numRequests = 20
	const key = "concurrent_test"

	results := make(chan bool, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/consume?key=%s&tokens=1", key), nil)
			w := httptest.NewRecorder()

			h.ConsumeTokens(w, req)

			var response ConsumeResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				results <- false
				return
			}

			results <- response.Allowed
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		if <-results {
			successCount++
		}
	}

	// Should have exactly 10 successes (bucket capacity)
	if successCount != 10 {
		t.Errorf("Expected exactly 10 successful requests, got %d", successCount)
	}
}
