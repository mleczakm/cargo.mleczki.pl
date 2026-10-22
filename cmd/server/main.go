package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"cargo.mleczki.pl/internal/articles"
	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/shop"
)

// defaultShopReservationHours is how long an unpaid order may hold shop stock.
const defaultShopReservationHours = 48

// shopReservationTTL reads SHOP_RESERVATION_TTL_HOURS; 0 disables expiry.
func shopReservationTTL() time.Duration {
	hours := defaultShopReservationHours
	if raw := strings.TrimSpace(os.Getenv("SHOP_RESERVATION_TTL_HOURS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			log.Printf("Ignoring invalid SHOP_RESERVATION_TTL_HOURS=%q, using %d", raw, defaultShopReservationHours)
		} else {
			hours = parsed
		}
	}
	return time.Duration(hours) * time.Hour
}

func main() {
	// Ensure data directories exist
	if err := os.MkdirAll("data/products", 0755); err != nil {
		log.Fatalf("Failed to create data/products directory: %v", err)
	}
	if err := os.MkdirAll("data/articles", 0755); err != nil {
		log.Fatalf("Failed to create data/articles directory: %v", err)
	}
	if err := os.MkdirAll("data/shop", 0755); err != nil {
		log.Fatalf("Failed to create data/shop directory: %v", err)
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

	// Initialize shop catalog and make sure every stocked SKU has a ledger row
	shopParser := shop.NewParser("data/shop")
	shopItems, shopErr := shopParser.LoadAll()
	if shopErr != nil {
		log.Printf("Shop catalog problems (affected items are hidden):\n%v", shopErr)
	}
	if err := readModels.Stock().SyncCatalog(ctx, shopItems); err != nil {
		log.Printf("Warning: failed to sync shop stock: %v", err)
	}

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
	server := NewServer(eventStore, readModels, projector, productParser, articleParser, shopParser, authManager)

	// Unpaid orders must not hold shop stock forever.
	if ttl := shopReservationTTL(); ttl > 0 {
		log.Printf("Unpaid orders holding shop stock expire after %s", ttl)
		go server.RunShopReservationSweeper(ctx, ttl, 10*time.Minute)
	}

	// Parcels ordered at Polkurier move their orders along.
	if server.polkurierOn() {
		go server.RunShipmentSync(ctx, 30*time.Minute)
	}

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
