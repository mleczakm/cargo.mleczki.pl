package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
)

func main() {
	// Ensure data directories exist
	if err := os.MkdirAll("data/products", 0755); err != nil {
		log.Fatalf("Failed to create data/products directory: %v", err)
	}
	if err := os.MkdirAll("db", 0755); err != nil {
		log.Fatalf("Failed to create db directory: %v", err)
	}

	// Initialize databases
	eventStore, err := eventstore.NewSQLiteEventStore("db/event_store.db")
	if err != nil {
		log.Fatalf("Failed to initialize event store: %v", err)
	}
	defer eventStore.Close()

	readModels, err := projections.NewReadModelsDB("db/read_models.db")
	if err != nil {
		log.Fatalf("Failed to initialize read models: %v", err)
	}
	defer readModels.Close()

	// Initialize product parser
	productParser := products.NewParser("data/products")

	// Initialize projector
	projector := projections.NewProjector(eventStore, readModels, "main")

	// Start projection runner in background
	ctx := context.Background()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := projector.Run(ctx); err != nil {
				log.Printf("Projection error: %v", err)
			}
		}
	}()

	// Create server
	server := NewServer(eventStore, readModels, productParser)

	// Create chi router
	r := chi.NewRouter()

	// Add middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Register routes
	server.RegisterRoutes(r)

	// Start HTTP server with timeouts
	addr := ":8080"
	log.Printf("Starting server on %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
