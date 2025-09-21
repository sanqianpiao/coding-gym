package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"monolith/internal/config"
	"monolith/internal/database"
	"monolith/internal/service"
)

type Server struct {
	userService *service.UserService
}

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
	log.Println("Starting Outbox Kafka Server...")

	cfg := config.Load()

	// Connect to database
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	repo := database.NewRepository(db)
	userService := service.NewUserService(repo, cfg.Kafka.Topic)

	server := &Server{
		userService: userService,
	}

	// Setup HTTP routes
	http.HandleFunc("/users", server.handleUsers)
	http.HandleFunc("/users/", server.handleUserByID)
	http.HandleFunc("/test/crash", server.handleCrashTest)
	http.HandleFunc("/health", server.handleHealth)

	// Start HTTP server
	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s", port)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Received shutdown signal, gracefully shutting down...")
	log.Println("Server stopped")
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path
	userIDStr := r.URL.Path[len("/users/"):]
	if userIDStr == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID format", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getUser(w, r, userID)
	case http.MethodPut:
		s.updateUser(w, r, userID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Failed to create user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	user, err := s.userService.GetUser(userID)
	if err != nil {
		if err.Error() == "user not found: "+userID.String() {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
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
		if err.Error() == "user not found: "+userID.String() {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to update user: %v", err)
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *Server) handleCrashTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CrashTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}

	user, err := s.userService.TestCrashScenario(req.Email, req.Name, req.CrashAfterDB)
	if err != nil {
		// This is expected for crash scenarios
		if req.CrashAfterDB {
			// Return the user data but with an error status to simulate crash
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted) // 202 to indicate partial success
			response := map[string]interface{}{
				"user":    user,
				"message": "User created but simulated crash occurred",
				"error":   err.Error(),
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		log.Printf("Failed to create user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"service": "outbox-kafka",
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
