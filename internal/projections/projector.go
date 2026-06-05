package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
)

// Projector handles event processing and updates read models
type Projector struct {
	eventStore     eventstore.EventStore
	readModels     *ReadModelsDB
	projectionName string
}

// NewProjector creates a new projector
func NewProjector(eventStore eventstore.EventStore, readModels *ReadModelsDB, projectionName string) *Projector {
	return &Projector{
		eventStore:     eventStore,
		readModels:     readModels,
		projectionName: projectionName,
	}
}

// Run processes all events since the last checkpoint
func (p *Projector) Run(ctx context.Context) error {
	lastVersion, err := p.readModels.GetCheckpoint(p.projectionName)
	if err != nil {
		return fmt.Errorf("failed to get checkpoint: %w", err)
	}

	events, err := p.eventStore.GetEventsSince(ctx, lastVersion)
	if err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}

	for _, event := range events {
		if err := p.processEvent(event); err != nil {
			log.Printf("Error processing event %s: %v", event.ID, err)
			continue
		}
		lastVersion = event.Version
	}

	return p.readModels.SaveCheckpoint(p.projectionName, lastVersion)
}

// processEvent handles a single event and updates the appropriate read model
func (p *Projector) processEvent(event *eventstore.Event) error {
	switch event.EventType {
	case "OrderPlaced":
		return p.handleOrderPlaced(event)
	case "OrderPaid":
		return p.handleOrderPaid(event)
	case "OrderCancelled":
		return p.handleOrderCancelled(event)
	case "UserRegistered":
		return p.handleUserRegistered(event)
	case "UserDetailsUpdated":
		return p.handleUserDetailsUpdated(event)
	case "UserDeletionRequested":
		return p.handleUserDeletionRequested(event)
	case "TransferReceived":
		return p.handleTransferReceived(event)
	case "TransferLinked":
		return p.handleTransferLinked(event)
	default:
		log.Printf("Unknown event type: %s", event.EventType)
	}
	return nil
}

// handleOrderPlaced creates or updates an order in the read model
func (p *Projector) handleOrderPlaced(event *eventstore.Event) error {
	var e domain.OrderPlacedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	itemsJSON, err := json.Marshal(e.Items)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO orders (id, user_id, items_json, total_amount, status, payment_method, start_date, end_date, rental_days, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		items_json = excluded.items_json,
		total_amount = excluded.total_amount,
		status = excluded.status,
		payment_method = excluded.payment_method,
		start_date = excluded.start_date,
		end_date = excluded.end_date,
		rental_days = excluded.rental_days,
		updated_at = excluded.updated_at
	`

	_, err = p.readModels.GetDB().Exec(query,
		e.OrderID,
		e.UserID,
		itemsJSON,
		e.TotalAmount,
		"pending_payment",
		e.PaymentMethod,
		e.StartDate,
		e.EndDate,
		e.RentalDays,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.Timestamp.Format("2006-01-02 15:04:05"),
	)

	return err
}

// handleOrderPaid updates the order status to paid
func (p *Projector) handleOrderPaid(event *eventstore.Event) error {
	var e domain.OrderPaidEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	UPDATE orders
	SET status = 'paid', updated_at = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query, e.Timestamp.Format("2006-01-02 15:04:05"), e.OrderID)
	return err
}

// handleOrderCancelled updates the order status to cancelled
func (p *Projector) handleOrderCancelled(event *eventstore.Event) error {
	var e domain.OrderCancelledEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	UPDATE orders
	SET status = 'cancelled', updated_at = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query, e.Timestamp.Format("2006-01-02 15:04:05"), e.OrderID)
	return err
}

// handleUserRegistered creates a user in the read model
func (p *Projector) handleUserRegistered(event *eventstore.Event) error {
	var e domain.UserRegisteredEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	INSERT INTO users (id, name, email, phone, address, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(email) DO UPDATE SET
		name = excluded.name,
		phone = excluded.phone,
		address = excluded.address,
		updated_at = excluded.updated_at
	`

	_, err := p.readModels.GetDB().Exec(query,
		e.UserID,
		e.Name,
		e.Email,
		e.Phone,
		e.Address,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.Timestamp.Format("2006-01-02 15:04:05"),
	)

	return err
}

// handleUserDetailsUpdated updates user information
func (p *Projector) handleUserDetailsUpdated(event *eventstore.Event) error {
	var e domain.UserDetailsUpdatedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	UPDATE users
	SET name = ?, email = ?, phone = ?, address = ?, updated_at = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query,
		e.Name,
		e.Email,
		e.Phone,
		e.Address,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.UserID,
	)

	return err
}

// handleUserDeletionRequested marks user for deletion
func (p *Projector) handleUserDeletionRequested(event *eventstore.Event) error {
	var e domain.UserDeletionRequestedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	UPDATE users
	SET deletion_requested_at = ?, updated_at = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.UserID,
	)

	return err
}

// handleTransferReceived creates a transfer record
func (p *Projector) handleTransferReceived(event *eventstore.Event) error {
	var e domain.TransferReceivedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	INSERT INTO transfers (id, date, sender, title, amount, status, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		date = excluded.date,
		sender = excluded.sender,
		title = excluded.title,
		amount = excluded.amount,
		status = excluded.status
	`

	_, err := p.readModels.GetDB().Exec(query,
		e.TransferID,
		e.Date,
		e.Sender,
		e.Title,
		e.Amount,
		"unmatched",
		e.Timestamp.Format("2006-01-02 15:04:05"),
	)

	return err
}

// handleTransferLinked links a transfer to an order
func (p *Projector) handleTransferLinked(event *eventstore.Event) error {
	var e domain.TransferLinkedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	// Update transfer status and link to order
	query := `
	UPDATE transfers
	SET status = 'matched', order_id = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query, e.OrderID, e.TransferID)
	return err
}
