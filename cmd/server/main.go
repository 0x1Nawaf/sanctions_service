package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

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
	log.Printf("Connected to database (pool max_open=%d max_idle=%d)", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)

	if cfg.EnablePprof {
		go func() {
			log.Println("pprof listening on :6060")
			if err := http.ListenAndServe(":6060", nil); err != nil {
				log.Printf("pprof server failed: %v", err)
			}
		}()
	}

	if cfg.ScreenShadowScore {
		// The table is a full pass over sanctions_names, so it is built in the
		// background: screening starts immediately and the candidate scorer
		// falls back to its static table of common name parts until this lands.
		go func() {
			if err := handler.LoadTokenWeights(db); err != nil {
				log.Printf("token weights load failed, shadow scoring will use fallback weights: %v", err)
			}
		}()
		log.Println("Shadow scoring enabled (reports shadow_score; does not affect results)")
	}

	healthH := handler.NewHealthHandler(db)
	recordsH := handler.NewRecordsHandler(db)
	screenH := handler.NewScreenHandler(db, cfg.ScreenUseLike, cfg.ScreenShadowScore)
	customListH := handler.NewCustomListHandler(db)
	historicalH := handler.NewHistoricalUpdatesHandler(db)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	r.Get("/health", healthH.Health)

	r.Group(func(r chi.Router) {
		r.Use(authmw.APIKeyAuth(cfg.APIKey))
		r.Post("/api/screen", screenH.Screen)
		r.Post("/api/screen/batch", screenH.ScreenBatch)
		r.Get("/api/records", recordsH.List)
		r.Get("/api/records/{id}", recordsH.Show)
		r.Post("/api/custom-lists", customListH.Upload)
		r.Get("/api/custom-lists", customListH.List)
		r.Delete("/api/custom-lists/{id}", customListH.Delete)
		r.Get("/api/historical_updates", historicalH.List)
	})

	addr := ":" + cfg.ServerPort
	log.Printf("Sanctions service starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
