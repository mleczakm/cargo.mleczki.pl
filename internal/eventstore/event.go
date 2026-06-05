package eventstore

import "encoding/json"

// Event represents a domain event in the event store
type Event struct {
	ID            string          `json:"id"`
	AggregateID   string          `json:"aggregate_id"`
	AggregateType string          `json:"aggregate_type"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	Version       int             `json:"version"`
	CreatedAt     string          `json:"created_at"`
}

// EventData is the interface that all domain events must implement
type EventData interface {
	EventType() string
}

// ToEvent converts EventData to an Event struct
func ToEvent(aggregateID, aggregateType string, data EventData, version int) (*Event, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Event{
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     data.EventType(),
		Payload:       payload,
		Version:       version,
	}, nil
}
