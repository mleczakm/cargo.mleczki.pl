package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
)

// Projector handles event processing and updates read models.
type Projector struct {
	eventStore     eventstore.EventStore
	readModels     *ReadModelsDB
	projectionName string
}

// NewProjector creates a new projector.
func NewProjector(eventStore eventstore.EventStore, readModels *ReadModelsDB, projectionName string) *Projector {
	return &Projector{
		eventStore:     eventStore,
		readModels:     readModels,
		projectionName: projectionName,
	}
}

// Run processes all events since the last checkpoint.
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
			if saveErr := p.readModels.SaveCheckpoint(p.projectionName, lastVersion); saveErr != nil {
				return fmt.Errorf("failed to save checkpoint: %w", saveErr)
			}
			return fmt.Errorf("failed to process event %s: %w", event.ID, err)
		}
		lastVersion = int(event.StreamPosition)
	}

	return p.readModels.SaveCheckpoint(p.projectionName, lastVersion)
}

// processEvent handles a single event and updates the appropriate read model.
func (p *Projector) processEvent(event *eventstore.Event) error {
	switch event.EventType {
	case "OrderPlaced":
		return p.handleOrderPlaced(event)
	case "OrderPaid":
		return p.handleOrderPaid(event)
	case "OrderCancelled":
		return p.handleOrderCancelled(event)
	case "OrderConfirmed":
		return p.handleOrderConfirmed(event)
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

// handleOrderPlaced creates or updates an order in the read model.
func (p *Projector) handleOrderPlaced(event *eventstore.Event) error {
	var eventData domain.OrderPlacedEvent
	if err := json.Unmarshal(event.Payload, &eventData); err != nil {
		return err
	}

	itemsJSON, err := json.Marshal(eventData.Items)
	if err != nil {
		return err
	}

	isFirstOrderInt := 0
	if eventData.IsFirstOrder {
		isFirstOrderInt = 1
	}

	// Order starts awaiting payment; pickup orders skip online payment and wait for admin.
	status := "awaiting_payment"
	if eventData.PaymentMethod == "cash_pickup" || eventData.PaymentMethod == "blik_pickup" {
		status = "paid"
	}

	query := `
	INSERT INTO orders (id, user_id, items_json, total_amount, status, payment_method, start_date, end_date, rental_days, created_at, updated_at, is_first_order, payment_code)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		items_json = excluded.items_json,
		total_amount = excluded.total_amount,
		status = excluded.status,
		payment_method = excluded.payment_method,
		start_date = excluded.start_date,
		end_date = excluded.end_date,
		rental_days = excluded.rental_days,
		updated_at = excluded.updated_at,
		is_first_order = excluded.is_first_order,
		payment_code = excluded.payment_code
	`

	var paymentCodePtr *string
	if eventData.PaymentCode != nil {
		paymentCodePtr = eventData.PaymentCode
	}

	_, err = p.readModels.GetDB().Exec(query,
		eventData.OrderID,
		eventData.UserID,
		itemsJSON,
		eventData.TotalAmount,
		status,
		eventData.PaymentMethod,
		eventData.StartDate,
		eventData.EndDate,
		eventData.RentalDays,
		eventData.Timestamp.Format("2006-01-02 15:04:05"),
		eventData.Timestamp.Format("2006-01-02 15:04:05"),
		isFirstOrderInt,
		paymentCodePtr,
	)
	if err != nil {
		return err
	}

	return p.blockOrderProductDates(eventData)
}

func (p *Projector) blockOrderProductDates(eventData domain.OrderPlacedEvent) error {
	if eventData.StartDate == "" || eventData.EndDate == "" || len(eventData.Items) == 0 {
		return nil
	}

	start, err := time.Parse("2006-01-02", eventData.StartDate)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}
	end, err := time.Parse("2006-01-02", eventData.EndDate)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	db := p.readModels.GetDB()
	for _, item := range eventData.Items {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			_, err = db.Exec(`
				INSERT INTO product_bookings (product_id, order_id, booked_date)
				VALUES (?, ?, ?)
			`, item.ProductID, eventData.OrderID, dateStr)
			if err != nil {
				return fmt.Errorf("product %s on %s: %w", item.ProductID, dateStr, err)
			}
		}
	}

	return nil
}

// handleOrderPaid updates the order status to paid, and auto-confirms for non-pickup payments.
func (p *Projector) handleOrderPaid(event *eventstore.Event) error {
	var e domain.OrderPaidEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	// Get payment method to determine if auto-confirmation is needed
	var paymentMethod string
	err := p.readModels.GetDB().QueryRow("SELECT payment_method FROM orders WHERE id = ?", e.OrderID).Scan(&paymentMethod)
	if err != nil {
		return err
	}

	// Auto-confirm for BLIK payments only
	// Pickup payments (cash_pickup) stay in 'paid' state awaiting manual confirmation
	newStatus := "paid"
	if paymentMethod == "blik" {
		newStatus = "confirmed"
	}

	query := `
	UPDATE orders
	SET status = ?, paid_at = ?, updated_at = ?
	WHERE id = ?
	`

	_, err = p.readModels.GetDB().Exec(query, newStatus, e.Timestamp.Format("2006-01-02 15:04:05"), e.Timestamp.Format("2006-01-02 15:04:05"), e.OrderID)
	return err
}

// handleOrderCancelled updates the order status to cancelled.
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
	if err != nil {
		return err
	}

	_, err = p.readModels.GetDB().Exec(`DELETE FROM product_bookings WHERE order_id = ?`, e.OrderID)
	return err
}

// handleOrderConfirmed updates the order status to confirmed.
func (p *Projector) handleOrderConfirmed(event *eventstore.Event) error {
	var e domain.OrderConfirmedEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	UPDATE orders
	SET status = 'confirmed', updated_at = ?
	WHERE id = ?
	`

	_, err := p.readModels.GetDB().Exec(query, e.Timestamp.Format("2006-01-02 15:04:05"), e.OrderID)
	return err
}

// handleUserRegistered creates a user in the read model.
func (p *Projector) handleUserRegistered(event *eventstore.Event) error {
	var e domain.UserRegisteredEvent
	if err := json.Unmarshal(event.Payload, &e); err != nil {
		return err
	}

	query := `
	INSERT INTO users (id, name, email, phone, address, is_admin, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(email) DO UPDATE SET
		name = excluded.name,
		phone = excluded.phone,
		address = excluded.address,
		is_admin = excluded.is_admin,
		updated_at = excluded.updated_at
	`

	_, err := p.readModels.GetDB().Exec(query,
		e.UserID,
		e.Name,
		e.Email,
		e.Phone,
		e.Address,
		0,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.Timestamp.Format("2006-01-02 15:04:05"),
	)

	return err
}

// handleUserDetailsUpdated updates user information.
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

// handleUserDeletionRequested marks user for deletion.
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

// handleTransferReceived creates a transfer record.
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

// handleTransferLinked links a transfer to an order.
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
