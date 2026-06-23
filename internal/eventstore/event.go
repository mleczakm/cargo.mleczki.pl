package eventstore

import (
	"encoding/json"
	"fmt"
)

// Event represents a domain event in the event store.
type Event struct {
	ID             string          `json:"id"`
	AggregateID    string          `json:"aggregateId"`
	AggregateType  string          `json:"aggregateType"`
	EventType      string          `json:"eventType"`
	Payload        json.RawMessage `json:"payload"`
	Version        int             `json:"version"`
	StreamPosition int64           `json:"-"`
	CreatedAt      string          `json:"createdAt"`
}

// EventData is the interface that all domain events must implement.
type EventData interface {
	EventType() string
}

// ToEvent converts EventData to an Event struct.
func ToEvent(aggregateID, aggregateType string, data EventData, version int) (*Event, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Event{
		ID:            fmt.Sprintf("%s-%s-v%d", aggregateID, data.EventType(), version),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     data.EventType(),
		Payload:       payload,
		Version:       version,
	}, nil
}
