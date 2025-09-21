package main

import (
	"flag"
	"log"
	"os"

	"monolith/internal/config"
	"monolith/internal/database"
)

func main() {
	var direction string
	flag.StringVar(&direction, "direction", "up", "Migration direction (up or down)")
	flag.Parse()

	// Override direction with command line argument if provided
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	if direction != "up" && direction != "down" {
		log.Fatal("Direction must be 'up' or 'down'")
	}

	cfg := config.Load()
	
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Printf("Running database migrations (%s)...", direction)
	
	if err := db.Migrate(direction); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migration completed successfully")
}
