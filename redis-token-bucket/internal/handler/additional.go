package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ResetBucket handles POST /api/reset - resets a bucket to full capacity
func (h *Handler) ResetBucket(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get key from query parameter
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "missing_key", "Key parameter is required")
		return
	}

	// Reset the bucket
	err := h.bucket.ResetBucket(ctx, key)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "bucket_error",
			fmt.Sprintf("Failed to reset bucket: %v", err))
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Bucket '%s' has been reset to full capacity", key),
		"key":     key,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// BulkConsume handles POST /api/bulk-consume - for testing purposes
func (h *Handler) BulkConsume(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := r.ParseForm(); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "parse_error", "Failed to parse request")
		return
	}

	// For simplicity, using query parameters for bulk test
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "missing_key", "Key parameter is required")
		return
	}

	// Simulate multiple rapid requests
	results := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		result, err := h.bucket.TakeTokens(ctx, key, 1)
		if err != nil {
			results[i] = map[string]interface{}{
				"attempt": i + 1,
				"error":   err.Error(),
			}
		} else {
			results[i] = map[string]interface{}{
				"attempt":          i + 1,
				"allowed":          result.Allowed,
				"remaining_tokens": result.RemainingTokens,
				"retry_after":      result.RetryAfter,
			}
		}
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Bulk consumption test completed",
		"key":     key,
		"results": results,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}
