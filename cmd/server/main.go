package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nnn/sanctions-service/internal/config"
	"github.com/nnn/sanctions-service/internal/database"
	"github.com/nnn/sanctions-service/internal/handler"
	authmw "github.com/nnn/sanctions-service/internal/middleware"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("Connected to database")

	healthH := handler.NewHealthHandler(db)
	recordsH := handler.NewRecordsHandler(db)
	screenH := handler.NewScreenHandler(db)
	customListH := handler.NewCustomListHandler(db)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	r.Get("/health", healthH.Health)

	r.Group(func(r chi.Router) {
		r.Use(authmw.APIKeyAuth(cfg.APIKey))
		r.Post("/api/screen", screenH.Screen)
		r.Get("/api/records", recordsH.List)
		r.Get("/api/records/{id}", recordsH.Show)
		r.Post("/api/custom-lists", customListH.Upload)
		r.Get("/api/custom-lists", customListH.List)
		r.Delete("/api/custom-lists/{id}", customListH.Delete)
	})

	addr := ":" + cfg.ServerPort
	log.Printf("Sanctions service starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
