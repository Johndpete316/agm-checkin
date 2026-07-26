package main

import (
	"log"
	"os"

	"johndpete316/agm-checkin-api/internal/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	database := db.Connect(dsn)
	if err := db.Migrate(database); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Println("migrations up to date")
}
