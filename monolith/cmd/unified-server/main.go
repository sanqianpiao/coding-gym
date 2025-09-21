package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	// Outbox-Kafka components
	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/service"

	// Token Bucket components
	"monolith/internal/bucket"
	"monolith/internal/handler"
)

type UnifiedServer struct {
	userService   *service.UserService
	bucketHandler *handler.Handler
}

// User management request types
type CreateUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UpdateUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type CrashTestRequest struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	CrashAfterDB bool   `json:"crash_after_db"`
}

func main() {
	log.Println("Starting Unified Monolith Server...")

	// Initialize Outbox-Kafka components
	cfg := config.Load()

	// Connect to database
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	repo := database.NewRepository(db)
	userService := service.NewUserService(repo, cfg.Kafka.Topic)

	// Initialize Redis Token Bucket components
	tb, err := bucket.NewRedisTokenBucket(&bucket.Config{
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       0,
	})
	if err != nil {
		log.Fatalf("Failed to initialize token bucket: %v", err)
	}
	defer tb.Close()

	bucketHandler := handler.NewHandler(tb)

	// Create unified server
	server := &UnifiedServer{
		userService:   userService,
		bucketHandler: bucketHandler,
	}

	// Setup routes
	r := mux.NewRouter()

	// Token Bucket API routes (prefix with /api/bucket)
	bucketAPI := r.PathPrefix("/api/bucket").Subrouter()
	bucketAPI.HandleFunc("/check", bucketHandler.CheckRate).Methods("GET")
	bucketAPI.HandleFunc("/consume", bucketHandler.ConsumeTokens).Methods("POST")
	bucketAPI.HandleFunc("/reset", bucketHandler.ResetBucket).Methods("POST")
	bucketAPI.HandleFunc("/bulk-consume", bucketHandler.BulkConsume).Methods("POST")

	// User Management API routes (prefix with /api/users)
	userAPI := r.PathPrefix("/api/users").Subrouter()
	userAPI.HandleFunc("", server.createUser).Methods("POST")
	userAPI.HandleFunc("/{id}", server.getUser).Methods("GET")
	userAPI.HandleFunc("/{id}", server.updateUser).Methods("PUT")

	// Test endpoints
	testAPI := r.PathPrefix("/api/test").Subrouter()
	testAPI.HandleFunc("/crash", server.crashTest).Methods("POST")

	// Health checks for both services
	r.HandleFunc("/health", server.health).Methods("GET")
	r.HandleFunc("/health/bucket", bucketHandler.Health).Methods("GET")
	r.HandleFunc("/health/users", server.userHealth).Methods("GET")

	// Start server
	port := getEnv("PORT", "8080")
	log.Printf("Unified server starting on port %s", port)
	log.Println("Available endpoints:")
	log.Println("  Token Bucket API: /api/bucket/*")
	log.Println("  User Management API: /api/users/*")
	log.Println("  Health: /health")

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

// User Management endpoints
func (s *UnifiedServer) createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}

	user, err := s.userService.CreateUser(req.Email, req.Name)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *UnifiedServer) getUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	userID, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := s.userService.GetUser(userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *UnifiedServer) updateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	userID, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}

	user, err := s.userService.UpdateUser(userID, req.Email, req.Name)
	if err != nil {
		log.Printf("Error updating user: %v", err)
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *UnifiedServer) crashTest(w http.ResponseWriter, r *http.Request) {
	var req CrashTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}

	// This endpoint simulates a crash scenario
	log.Printf("Simulating crash scenario: crashAfterDB=%v", req.CrashAfterDB)
	_, err := s.userService.TestCrashScenario(req.Email, req.Name, req.CrashAfterDB)
	if err != nil {
		log.Printf("Crash test completed with expected error: %v", err)
		http.Error(w, "Crash simulation completed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Crash test completed"))
}

// Health check endpoints
func (s *UnifiedServer) health(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "healthy",
		"services": map[string]string{
			"token-bucket": "healthy",
			"user-service": "healthy",
		},
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *UnifiedServer) userHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "user-management",
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
