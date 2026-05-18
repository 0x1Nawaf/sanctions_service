package main

import (
	"log"
	"os"

	"github.com/nnn/sanctions-service/internal/config"
	"github.com/nnn/sanctions-service/internal/database"
	"github.com/nnn/sanctions-service/internal/seeder"
)

func main() {
	cfg := config.Load()

	jsonPath := cfg.SanctionsJSONPath
	if len(os.Args) > 1 {
		jsonPath = os.Args[1]
	}

	if jsonPath == "" {
		log.Fatal("No JSON path provided. Pass as argument or set SANCTIONS_JSON_PATH in .env")
	}

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		log.Fatalf("JSON file not found: %s", jsonPath)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("Connected to database")

	s := seeder.New(db)
	if err := s.Run(jsonPath); err != nil {
		log.Fatalf("Seeder failed: %v", err)
	}
}
