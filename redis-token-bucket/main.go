package main

import (
	"fmt"
	"log"
	"net/http"

	"redis-token-bucket/internal/bucket"
	"redis-token-bucket/internal/handler"

	"github.com/gorilla/mux"
)

func main() {
	// Initialize Redis-backed token bucket
	tb, err := bucket.NewRedisTokenBucket(&bucket.Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       0,
	})
	if err != nil {
		log.Fatalf("Failed to initialize token bucket: %v", err)
	}
	defer tb.Close()

	// Create HTTP handlers
	h := handler.NewHandler(tb)

	// Setup routes
	r := mux.NewRouter()
	r.HandleFunc("/api/check", h.CheckRate).Methods("GET")
	r.HandleFunc("/api/consume", h.ConsumeTokens).Methods("POST")
	r.HandleFunc("/api/reset", h.ResetBucket).Methods("POST")
	r.HandleFunc("/api/bulk-consume", h.BulkConsume).Methods("POST")
	r.HandleFunc("/health", h.Health).Methods("GET")

	// Start server
	fmt.Println("Server starting on :8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}
