package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"cargo.mleczki.pl/internal/articles"
	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
)

func main() {
	// Ensure data directories exist
	if err := os.MkdirAll("data/products", 0755); err != nil {
		log.Fatalf("Failed to create data/products directory: %v", err)
	}
	if err := os.MkdirAll("data/articles", 0755); err != nil {
		log.Fatalf("Failed to create data/articles directory: %v", err)
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

	// Initialize auth manager for admin user creation
	authManager := auth.NewAuthManager(readModels.GetDB(), eventStore)

	// Ensure admin user exists
	ctx := context.Background()
	password, err := authManager.EnsureAdminUser(ctx)
	if err != nil {
		log.Printf("Warning: Failed to ensure admin user: %v", err)
	} else if password != "" {
		log.Printf("========================================")
		log.Printf("ADMIN USER CREATED")
		log.Printf("Email: admin@example.com")
		log.Printf("Password: %s", password)
		log.Printf("========================================")
	}

	// Initialize product parser
	productParser := products.NewParser("data/products")

	// Initialize article parser
	articleParser := articles.NewParser("data/articles")

	// Initialize projector
	projector := projections.NewProjector(eventStore, readModels, "main")

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
	server := NewServer(eventStore, readModels, projector, productParser, articleParser, authManager)

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
