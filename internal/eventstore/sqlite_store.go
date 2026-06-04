package eventstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteEventStore implements EventStore using SQLite
type SQLiteEventStore struct {
	db *sql.DB
}

// NewSQLiteEventStore creates a new SQLite event store
func NewSQLiteEventStore(dbPath string) (*SQLiteEventStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	store := &SQLiteEventStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the events table if it doesn't exist
func (s *SQLiteEventStore) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		aggregate_id TEXT NOT NULL,
		aggregate_type TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload BLOB NOT NULL,
		version INTEGER NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id);
	CREATE INDEX IF NOT EXISTS idx_events_aggregate_type ON events(aggregate_type);
	CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type);
	CREATE INDEX IF NOT EXISTS idx_events_version ON events(version);
	`

	_, err := s.db.Exec(query)
	return err
}

// Save appends a new event to the store
func (s *SQLiteEventStore) Save(ctx context.Context, event *Event) error {
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	query := `
	INSERT INTO events (id, aggregate_id, aggregate_type, event_type, payload, version, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		event.ID,
		event.AggregateID,
		event.AggregateType,
		event.EventType,
		event.Payload,
		event.Version,
		event.CreatedAt,
	)

	return err
}

// GetEvents retrieves all events for a specific aggregate
func (s *SQLiteEventStore) GetEvents(ctx context.Context, aggregateID string) ([]*Event, error) {
	query := `
	SELECT id, aggregate_id, aggregate_type, event_type, payload, version, created_at
	FROM events
	WHERE aggregate_id = ?
	ORDER BY version ASC
	`

	rows, err := s.db.QueryContext(ctx, query, aggregateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.Payload,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// GetEventsByType retrieves all events of a specific type
func (s *SQLiteEventStore) GetEventsByType(ctx context.Context, eventType string) ([]*Event, error) {
	query := `
	SELECT id, aggregate_id, aggregate_type, event_type, payload, version, created_at
	FROM events
	WHERE event_type = ?
	ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.Payload,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// GetAllEvents retrieves all events from the store
func (s *SQLiteEventStore) GetAllEvents(ctx context.Context) ([]*Event, error) {
	query := `
	SELECT id, aggregate_id, aggregate_type, event_type, payload, version, created_at
	FROM events
	ORDER BY version ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.Payload,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// GetEventsSince retrieves all events after a specific version
func (s *SQLiteEventStore) GetEventsSince(ctx context.Context, version int) ([]*Event, error) {
	query := `
	SELECT id, aggregate_id, aggregate_type, event_type, payload, version, created_at
	FROM events
	WHERE version > ?
	ORDER BY version ASC
	`

	rows, err := s.db.QueryContext(ctx, query, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.Payload,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// Close closes the database connection
func (s *SQLiteEventStore) Close() error {
	return s.db.Close()
}
