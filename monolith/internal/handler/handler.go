package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"monolith/internal/bucket"
)

// Handler contains the HTTP handlers for the token bucket API
type Handler struct {
	bucket *bucket.RedisTokenBucket
}

// NewHandler creates a new HTTP handler
func NewHandler(tb *bucket.RedisTokenBucket) *Handler {
	return &Handler{
		bucket: tb,
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// CheckRateResponse represents the response for checking bucket state
type CheckRateResponse struct {
	*bucket.BucketState
	Success bool `json:"success"`
}

// ConsumeResponse represents the response for token consumption
type ConsumeResponse struct {
	*bucket.TokenResult
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// writeErrorResponse writes an error response
func (h *Handler) writeErrorResponse(w http.ResponseWriter, statusCode int, err string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   err,
		Message: message,
	})
}

// writeJSONResponse writes a JSON response
func (h *Handler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// CheckRate handles GET /api/check - returns current bucket state
func (h *Handler) CheckRate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get key from query parameter
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "missing_key", "Key parameter is required")
		return
	}

	// Get bucket state
	state, err := h.bucket.GetBucketState(ctx, key)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "bucket_error",
			fmt.Sprintf("Failed to get bucket state: %v", err))
		return
	}

	response := CheckRateResponse{
		BucketState: state,
		Success:     true,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// ConsumeTokens handles POST /api/consume - attempts to consume tokens
func (h *Handler) ConsumeTokens(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get key from query parameter
	key := r.URL.Query().Get("key")
	if key == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "missing_key", "Key parameter is required")
		return
	}

	// Get tokens from query parameter (default to 1)
	tokensStr := r.URL.Query().Get("tokens")
	if tokensStr == "" {
		tokensStr = "1"
	}

	tokens, err := strconv.ParseFloat(tokensStr, 64)
	if err != nil || tokens <= 0 {
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid_tokens",
			"Tokens parameter must be a positive number")
		return
	}

	// Attempt to consume tokens
	result, err := h.bucket.TakeTokens(ctx, key, tokens)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "bucket_error",
			fmt.Sprintf("Failed to consume tokens: %v", err))
		return
	}

	response := ConsumeResponse{
		TokenResult: result,
		Success:     result.Allowed,
	}

	if result.Allowed {
		response.Message = fmt.Sprintf("Successfully consumed %.1f tokens", tokens)
	} else {
		response.Message = "Rate limit exceeded"
	}

	// Set appropriate HTTP status code
	statusCode := http.StatusOK
	if !result.Allowed {
		statusCode = http.StatusTooManyRequests
		// Add Retry-After header if available
		if result.RetryAfter > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter))
		}
	}

	h.writeJSONResponse(w, statusCode, response)
}

// Health handles GET /health - health check endpoint
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"service":   "redis-token-bucket",
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}
