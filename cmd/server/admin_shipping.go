package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	polkurier "github.com/mleczakm/polkurier-go"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/email"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/shipping"
)

func orderRedirect(w http.ResponseWriter, r *http.Request, orderID, kind, message string) {
	http.Redirect(w, r, "/admin/order/"+url.PathEscape(orderID)+"?"+kind+"="+url.QueryEscape(message)+"#przesylka", http.StatusSeeOther)
}

// shipmentView adds what the page needs to a stored shipment.
type shipmentView struct {
	*shipping.Shipment
	Cost domain.Money
}

func shipmentViews(list []*shipping.Shipment) []shipmentView {
	views := make([]shipmentView, 0, len(list))
	for _, sh := range list {
		views = append(views, shipmentView{Shipment: sh, Cost: domain.PLN(sh.PriceGross)})
	}
	return views
}

type quoteRow struct {
	Name      string
	Code      string
	Cost      domain.Money
	Available bool
	Message   string
	Chosen    bool
}

// handleAdminOrder shows an order with its delivery and parcels.
func (s *Server) handleAdminOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	order, err := s.readModels.GetOrderDetail(orderID)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("Failed to load order %s: %v", orderID, err)
		http.Error(w, "Failed to load order", http.StatusInternalServerError)
		return
	}

	items := parseOrderItems(order.ItemsJSON)
	shipments, err := s.shipments().ForOrder(r.Context(), orderID)
	if err != nil {
		log.Printf("Failed to load shipments of %s: %v", orderID, err)
	}
	cfg := s.shippingConfig()

	hasActive := false
	for _, sh := range shipments {
		hasActive = hasActive || sh.Active()
	}
	paidState := order.Status == string(domain.StatusPaid) || order.Status == string(domain.StatusConfirmed)

	data := map[string]interface{}{
		"Title":           "Zamówienie " + order.ID,
		"Order":           order,
		"Items":           items,
		"StatusLabel":     orderStatusLabel(order.Status, order.PaymentMethod),
		"Delivery":        order.Delivery,
		"Shipments":       shipmentViews(shipments),
		"ShippingEnabled": s.polkurierOn(),
		"HasActive":       hasActive,
		"CanRealize":      order.Status == string(domain.StatusConfirmed),
		"AdminNotice":     r.URL.Query().Get("notice"),
		"AdminError":      r.URL.Query().Get("error"),
		"IsAdmin":         true,
	}
	if s.shipping != nil && s.shipping.Config != nil {
		if _, err := s.shipping.Config.Load(); err != nil {
			data["ConfigError"] = err.Error()
		}
	}

	canShip := s.polkurierOn() && order.Delivery.IsShipped() && paidState && !hasActive
	data["CanShip"] = canShip
	if canShip {
		s.addShipmentForm(r.Context(), data, cfg, order, items)
	}
	s.renderTemplate(w, r, "admin_order.html", data)
}

// addShipmentForm fills the "send a parcel" form: the sizing from the goods, the
// courier prices and the pickup windows.
func (s *Server) addShipmentForm(ctx context.Context, data map[string]interface{}, cfg shipping.Config, order *projections.OrderDetail, items []domain.OrderItem) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	d := order.Delivery
	dest := shipping.Destination{
		Method: d.Method, Person: firstNonEmpty(d.Person, order.UserName), Email: firstNonEmpty(d.Email, order.UserEmail), Phone: firstNonEmpty(d.Phone, order.UserPhone),
		Street: d.Street, Postcode: d.Postcode, City: d.City, PointID: d.PointID,
	}
	in := s.shipping.DefaultInput(ctx, cfg, order.ID, dest, s.goodsOf(items), goodsValue(items))
	data["ShipInput"] = in
	data["SenderError"] = cfg.Sender.Missing()

	if carriers, err := s.shipping.Carriers(ctx); err == nil {
		data["Carriers"] = carriers
		care := make(map[string]bool, len(carriers))
		for _, c := range carriers {
			care[c.ServiceCode] = shipping.SupportsHandleWithCare(c)
		}
		data["CareSupported"] = care
	}

	pack := polkurier.Pack{Length: in.LengthCm, Width: in.WidthCm, Height: in.HeightCm, Weight: in.WeightKg}
	quotes, err := s.shipping.Quote(ctx, cfg.Sender, pack, in.Recipient.Postcode, in.Insurance)
	if err != nil {
		data["QuoteError"] = err.Error()
	}
	rows := make([]quoteRow, 0, len(quotes))
	for _, q := range quotes {
		rows = append(rows, quoteRow{
			Name: q.ServiceName, Code: q.ServiceCode, Cost: domain.PLN(q.FinalGross().Grosz()),
			Available: bool(q.Available), Message: q.UnavailableMessage, Chosen: strings.EqualFold(q.ServiceCode, in.Carrier),
		})
	}
	data["Quotes"] = rows
	for _, row := range rows {
		if row.Chosen {
			data["ChosenCost"] = row.Cost
			data["ShippingMargin"] = d.Cost.Sub(row.Cost)
		}
	}

	if !in.DropOff && in.Carrier != "" {
		if days, err := s.shipping.PickupDays(ctx, in.Carrier, cfg.Sender); err == nil {
			data["PickupDays"] = days
		}
	}
}

// handleAdminShipmentCreate orders a parcel at Polkurier for an order.
func (s *Server) handleAdminShipmentCreate(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	order, err := s.readModels.GetOrderDetail(orderID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fail := func(err error) { orderRedirect(w, r, orderID, "error", err.Error()) }

	if !order.Delivery.IsShipped() {
		fail(errors.New("to zamówienie jest do odbioru osobistego"))
		return
	}
	if order.Status != string(domain.StatusPaid) && order.Status != string(domain.StatusConfirmed) {
		fail(errors.New("przesyłkę można nadać dopiero dla opłaconego zamówienia"))
		return
	}
	if existing, _ := s.shipments().ForOrder(r.Context(), orderID); len(existing) > 0 {
		for _, sh := range existing {
			if sh.Active() {
				fail(errors.New("to zamówienie ma już aktywną przesyłkę — anuluj ją, zanim nadasz nową"))
				return
			}
		}
	}

	f := func(name string) string { return strings.TrimSpace(r.FormValue(name)) }
	atoi := func(name string) int { v, _ := strconv.Atoi(f(name)); return v }
	num := func(name string) float64 {
		v, _ := strconv.ParseFloat(strings.ReplaceAll(f(name), ",", "."), 64)
		return v
	}
	in := shipping.ShipmentInput{
		OrderID: orderID, Carrier: f("carrier"), LengthCm: atoi("length"), WidthCm: atoi("width"), HeightCm: atoi("height"), WeightKg: num("weight"),
		Insurance: num("insurance"), HandleWithCare: r.FormValue("handle_with_care") == "1", Description: f("description"), DropOff: r.FormValue("drop_off") == "1",
		Recipient: polkurier.Address{
			Company: f("r_company"), Person: f("r_person"), Street: f("r_street"), HouseNumber: f("r_house"), FlatNumber: f("r_flat"),
			Postcode: f("r_postcode"), City: f("r_city"), Email: f("r_email"), Phone: f("r_phone"), Country: "PL", PointID: strings.ToUpper(f("r_point")),
		},
	}
	if window := f("pickup"); window != "" && !in.DropOff {
		// "2026-09-24|10:00|14:00"
		if parts := strings.Split(window, "|"); len(parts) == 3 {
			in.PickupDate, in.PickupFrom, in.PickupTo = parts[0], parts[1], parts[2]
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	sh, err := s.shipping.CreateShipment(ctx, s.shippingConfig(), in)
	if err != nil {
		fail(err)
		return
	}

	notice := fmt.Sprintf("Nadano przesyłkę %s (list przewozowy %s, koszt %s). Pobierz etykietę.", sh.ExternalNumber, sh.Waybill, domain.PLN(sh.PriceGross))
	if r.FormValue("mark_shipped") == "1" {
		switch err := s.realizeOrder(ctx, orderID); {
		case err == nil:
			notice += " Zamówienie oznaczone jako wysłane, towar zszedł ze stanu."
			s.notifyShipped(ctx, order, sh)
		case errors.Is(err, errOrderNotConfirmed):
			notice += " Zamówienie nie jest jeszcze potwierdzone — oznacz je jako zrealizowane po potwierdzeniu."
		default:
			notice += " Nie udało się oznaczyć zamówienia jako wysłane: " + err.Error()
		}
	}
	orderRedirect(w, r, orderID, "notice", notice)
}

func (s *Server) shipmentFromURL(r *http.Request) (*shipping.Shipment, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
	if err != nil {
		return nil, shipping.ErrShipmentNotFound
	}
	return s.shipments().ByID(r.Context(), id)
}

func (s *Server) handleAdminShipmentLabel(w http.ResponseWriter, r *http.Request) {
	sh, err := s.shipmentFromURL(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pdf, err := s.shipping.Label(ctx, sh)
	if err != nil {
		orderRedirect(w, r, sh.OrderID, "error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="etykieta-`+safeFilename(sh.OrderID)+`.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func safeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, s)
}

func (s *Server) handleAdminShipmentRefresh(w http.ResponseWriter, r *http.Request) {
	sh, err := s.shipmentFromURL(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	after, err := s.shipping.Refresh(ctx, sh)
	if err != nil {
		orderRedirect(w, r, sh.OrderID, "error", err.Error())
		return
	}
	s.onShipmentChanged(ctx, sh, after)
	orderRedirect(w, r, sh.OrderID, "notice", "Zaktualizowano status przesyłki: "+after.Status+".")
}

func (s *Server) handleAdminShipmentCancel(w http.ResponseWriter, r *http.Request) {
	sh, err := s.shipmentFromURL(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.shipping.Cancel(ctx, sh); err != nil {
		orderRedirect(w, r, sh.OrderID, "error", err.Error())
		return
	}
	orderRedirect(w, r, sh.OrderID, "notice", "Anulowano przesyłkę "+sh.ExternalNumber+".")
}

// onShipmentChanged moves the order along when the courier reports progress:
// a parcel on its way means the goods have left the shelf.
func (s *Server) onShipmentChanged(ctx context.Context, before, after *shipping.Shipment) {
	switch {
	case shipping.InTransit(after):
		err := s.realizeOrder(ctx, after.OrderID)
		if err != nil && !errors.Is(err, errOrderNotConfirmed) {
			log.Printf("Parcel %s is on its way but order %s could not be marked as sent: %v", after.ExternalNumber, after.OrderID, err)
		}
	case shipping.Problem(after):
		log.Printf("Parcel %s of order %s has a problem: %s", after.ExternalNumber, after.OrderID, after.Status)
	}
}

// RunShipmentSync keeps parcel statuses up to date.
func (s *Server) RunShipmentSync(ctx context.Context, every time.Duration) {
	s.shipping.RunSync(ctx, every, s.onShipmentChanged)
}

// notifyShipped tells the customer that the parcel is on its way.
func (s *Server) notifyShipped(ctx context.Context, order *projections.OrderDetail, sh *shipping.Shipment) {
	if s.mailer == nil || !s.mailer.Configured() || order.UserEmail == "" {
		return
	}
	where := ""
	if order.Delivery != nil {
		where = order.Delivery.Where()
	}
	body, err := email.RenderOrderShipped(email.OrderShipped{
		OrderID: order.ID, Name: order.UserName, Carrier: sh.Carrier, Waybill: sh.Waybill, TrackingURL: sh.TrackingURL, Where: where,
	})
	if err != nil {
		log.Printf("Failed to render shipping email for %s: %v", order.ID, err)
		return
	}
	go func(ctx context.Context) {
		if err := s.mailer.SendEmail(ctx, email.DefaultSender(), []email.EmailRecipient{{Email: order.UserEmail}}, email.OrderShippedSubject, body); err != nil {
			log.Printf("Failed to send shipping email for %s: %v", order.ID, err)
		}
	}(context.WithoutCancel(ctx))
}

// ordersToShip picks the paid orders that have to be posted and have no parcel yet.
func (s *Server) ordersToShip(ctx context.Context, orders []map[string]interface{}) []map[string]interface{} {
	var queue []map[string]interface{}
	for _, order := range orders {
		delivery, _ := order["Delivery"].(*domain.Delivery)
		status, _ := order["Status"].(string)
		if !delivery.IsShipped() || (status != string(domain.StatusPaid) && status != string(domain.StatusConfirmed)) {
			continue
		}
		id, _ := order["ID"].(string)
		sent := false
		if shipments, err := s.shipments().ForOrder(ctx, id); err == nil {
			for _, sh := range shipments {
				sent = sent || sh.Active()
			}
		}
		if !sent {
			queue = append(queue, order)
		}
	}
	return queue
}
