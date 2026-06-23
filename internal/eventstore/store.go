package eventstore

import (
	"context"
)

// EventStore defines the interface for storing and retrieving events.
type EventStore interface {
	// Save appends a new event to the store
	Save(ctx context.Context, event *Event) error

	// GetEvents retrieves all events for a specific aggregate
	GetEvents(ctx context.Context, aggregateID string) ([]*Event, error)

	// GetEventsByType retrieves all events of a specific type
	GetEventsByType(ctx context.Context, eventType string) ([]*Event, error)

	// GetAllEvents retrieves all events from the store (for projections)
	GetAllEvents(ctx context.Context) ([]*Event, error)

	// GetEventsSince retrieves all events after a specific stream position (SQLite rowid).
	GetEventsSince(ctx context.Context, position int) ([]*Event, error)
}
