package shipping

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	polkurier "github.com/mleczakm/polkurier-go"

	"cargo.mleczki.pl/internal/domain"
)

// ErrNotConfigured is returned when no Polkurier credentials are set.
var ErrNotConfigured = errors.New("integracja z Polkurierem nie jest skonfigurowana (ustaw POLKURIER_LOGIN i POLKURIER_TOKEN)")

// ErrPointNotFound is returned for unknown locker / point codes.
var ErrPointNotFound = errors.New("nie znaleziono takiego punktu odbioru")

// Service talks to Polkurier and keeps the shipments table up to date.
type Service struct {
	Config *ConfigFile
	Store  *Store
	Log    *slog.Logger

	clientMu sync.RWMutex
	client   *polkurier.Client

	mu        sync.Mutex
	carriers  []polkurier.Carrier
	carriersT time.Time
	quotes    map[string]quoteEntry
	points    map[string]pointsEntry
}

type quoteEntry struct {
	at   time.Time
	list []polkurier.Valuation
}

type pointsEntry struct {
	at   time.Time
	list []Point
}

// Configure sets or clears the Polkurier client. apiURL overrides the endpoint (tests, local proxies).
func (s *Service) Configure(login, token string, sandbox bool, apiURL, version string) {
	var client *polkurier.Client
	if strings.TrimSpace(login) != "" && strings.TrimSpace(token) != "" {
		opts := []polkurier.Option{polkurier.WithPlatform("cargo.mleczki.pl", version)}
		if sandbox {
			opts = append(opts, polkurier.WithSandbox())
		}
		if apiURL != "" {
			opts = append(opts, polkurier.WithBaseURL(apiURL))
		}
		client = polkurier.New(strings.TrimSpace(login), strings.TrimSpace(token), opts...)
	}
	s.clientMu.Lock()
	s.client = client
	s.clientMu.Unlock()
	s.mu.Lock()
	s.carriers, s.carriersT, s.quotes, s.points = nil, time.Time{}, nil, nil
	s.mu.Unlock()
}

func (s *Service) api() *polkurier.Client {
	if s == nil {
		return nil
	}
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.client
}

// Configured reports whether Polkurier can be used.
func (s *Service) Configured() bool { return s.api() != nil }

func (s *Service) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Carriers lists the couriers available on the account (cached for 12 hours).
func (s *Service) Carriers(ctx context.Context) ([]polkurier.Carrier, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	if time.Since(s.carriersT) < 12*time.Hour && len(s.carriers) > 0 {
		defer s.mu.Unlock()
		return s.carriers, nil
	}
	s.mu.Unlock()
	list, err := client.AvailableCarriers(ctx, true, "")
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	s.mu.Lock()
	s.carriers, s.carriersT = list, time.Now()
	s.mu.Unlock()
	return list, nil
}

// SupportsHandleWithCare reports whether a courier offers the optional handling service.
func SupportsHandleWithCare(carrier polkurier.Carrier) bool {
	if carrier.Details == nil {
		return false
	}
	return bool(carrier.Details.Services[polkurier.ServiceHandleWithCare].Available)
}

// Good is one kind of goods in an order, as far as packing is concerned.
type Good struct {
	Quantity int
	WeightG  int // 0 = use the default item weight
	LengthCm int // 0 = fits the default box
	WidthCm  int
	HeightCm int
}

// PackFor sizes one parcel for goods: the weights add up, a good with its own
// dimensions grows the default box.
func PackFor(cfg Config, goods []Good) polkurier.Pack {
	grams := cfg.Parcel.PackagingWeightG
	length, width, height := cfg.Parcel.LengthCm, cfg.Parcel.WidthCm, cfg.Parcel.HeightCm
	for _, good := range goods {
		w := good.WeightG
		if w <= 0 {
			w = cfg.Parcel.DefaultItemWeightG
		}
		grams += w * max(good.Quantity, 1)
		length, width, height = max(length, good.LengthCm), max(width, good.WidthCm), max(height, good.HeightCm)
	}
	if grams <= 0 {
		grams = 1000
	}
	return polkurier.Pack{
		Length: length, Width: width, Height: height,
		Weight: float64((grams+99)/100) / 10, // round up to 0.1 kg
		Amount: 1, Type: polkurier.PackStandard,
	}
}

// Quote prices a parcel with every courier (cached for an hour per parcel and postcodes).
// Valuations are sorted by final gross price.
func (s *Service) Quote(ctx context.Context, sender Sender, pack polkurier.Pack, recipientPostcode string, insurance float64) ([]polkurier.Valuation, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	key := fmt.Sprintf("%d/%d/%d/%.1f/%s/%s/%.0f", pack.Length, pack.Width, pack.Height, pack.Weight, sender.Postcode, recipientPostcode, insurance)
	s.mu.Lock()
	if e, ok := s.quotes[key]; ok && time.Since(e.at) < time.Hour {
		s.mu.Unlock()
		return e.list, nil
	}
	s.mu.Unlock()
	req := polkurier.ValuationRequest{OrderRequest: polkurier.OrderRequest{
		ShipmentType: polkurier.ShipmentBox, Packs: []polkurier.Pack{pack}, Insurance: insurance,
	}}
	if sender.Postcode != "" {
		req.Sender = &polkurier.Address{Postcode: sender.Postcode, Country: "PL"}
	}
	if recipientPostcode != "" {
		req.Recipient = &polkurier.Address{Postcode: recipientPostcode, Country: "PL"}
	}
	list, err := client.OrderValuationV2(ctx, req)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].FinalGross() < list[j].FinalGross() })
	s.mu.Lock()
	if s.quotes == nil {
		s.quotes = map[string]quoteEntry{}
	}
	s.quotes[key] = quoteEntry{at: time.Now(), list: list}
	s.mu.Unlock()
	return list, nil
}

// PriceFor finds the valuation of one courier.
func PriceFor(list []polkurier.Valuation, carrier string) (polkurier.Valuation, bool) {
	for _, v := range list {
		if strings.EqualFold(v.ServiceCode, carrier) {
			return v, true
		}
	}
	return polkurier.Valuation{}, false
}

// Point is a parcel locker or pickup point offered at checkout.
type Point struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Street      string `json:"-"`
	Postcode    string `json:"-"`
	City        string `json:"-"`
}

// SearchPoints finds points of a courier where parcels can be collected (by locker code, town, street or postcode).
func (s *Service) SearchPoints(ctx context.Context, carrier, query string) ([]Point, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return nil, nil
	}
	key := strings.ToLower(carrier + "|" + query)
	s.mu.Lock()
	if e, ok := s.points[key]; ok && time.Since(e.at) < 10*time.Minute {
		s.mu.Unlock()
		return e.list, nil
	}
	s.mu.Unlock()
	pts, err := client.GetCourierPoints(ctx, polkurier.PointQuery{
		Couriers: []string{carrier}, SearchQuery: query, Functions: []polkurier.PointFunction{polkurier.PointCollect}, Limit: 15, Page: 1,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Point, 0, len(pts))
	for _, p := range pts {
		if bool(p.Available) {
			out = append(out, toPoint(p))
		}
	}
	s.mu.Lock()
	if s.points == nil {
		s.points = map[string]pointsEntry{}
	}
	s.points[key] = pointsEntry{at: time.Now(), list: out}
	s.mu.Unlock()
	return out, nil
}

func toPoint(p polkurier.CourierPoint) Point {
	addr := p.Address
	if addr == "" {
		addr = strings.TrimSpace(p.Zip + " " + p.City + ", " + p.Street)
	}
	return Point{ID: string(p.ID), Label: string(p.ID) + " — " + addr, Description: p.Description, Street: p.Street, Postcode: p.Zip, City: p.City}
}

// LookupPoint returns a point by ID, or ErrPointNotFound.
func (s *Service) LookupPoint(ctx context.Context, carrier, id string) (*Point, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	pts, err := client.GetCourierPoints(ctx, polkurier.PointQuery{Couriers: []string{carrier}, ID: strings.ToUpper(strings.TrimSpace(id))})
	if err != nil {
		return nil, err
	}
	for _, p := range pts {
		if strings.EqualFold(string(p.ID), id) {
			pt := toPoint(p)
			return &pt, nil
		}
	}
	return nil, ErrPointNotFound
}

// ShipmentInput is what the admin confirms before a parcel is ordered.
type ShipmentInput struct {
	OrderID        string
	Carrier        string
	LengthCm       int
	WidthCm        int
	HeightCm       int
	WeightKg       float64
	Insurance      float64 // PLN, 0 = none
	HandleWithCare bool
	Description    string
	Recipient      polkurier.Address
	DropOff        bool // true: drop the parcel at a point/locker; false: courier pickup
	PickupDate     string
	PickupFrom     string
	PickupTo       string
}

func (in ShipmentInput) validate() error {
	switch {
	case in.OrderID == "":
		return errors.New("brak zamówienia")
	case in.Carrier == "":
		return errors.New("wybierz przewoźnika")
	case in.LengthCm <= 0 || in.WidthCm <= 0 || in.HeightCm <= 0 || in.WeightKg <= 0:
		return errors.New("podaj wymiary i wagę paczki")
	case strings.TrimSpace(in.Recipient.Person) == "" || strings.TrimSpace(in.Recipient.Phone) == "" || strings.TrimSpace(in.Recipient.Email) == "":
		return errors.New("odbiorca potrzebuje imienia i nazwiska, telefonu i e-maila")
	case in.Recipient.PointID == "" && (in.Recipient.Street == "" || in.Recipient.HouseNumber == "" || in.Recipient.Postcode == "" || in.Recipient.City == ""):
		return errors.New("uzupełnij adres odbiorcy albo kod paczkomatu")
	}
	return nil
}

//nolint:nilnil // A nil service map means no optional services were requested.
func (s *Service) selectedServices(ctx context.Context, carrierCode string, handleWithCare bool) (map[polkurier.Service]bool, error) {
	if !handleWithCare {
		return nil, nil
	}
	carriers, err := s.Carriers(ctx)
	if err != nil {
		return nil, fmt.Errorf("nie można sprawdzić usług dodatkowych przewoźnika: %w", err)
	}
	for _, carrier := range carriers {
		if strings.EqualFold(carrier.ServiceCode, carrierCode) && SupportsHandleWithCare(carrier) {
			return map[polkurier.Service]bool{polkurier.ServiceHandleWithCare: true}, nil
		}
	}
	return nil, errors.New("wybrany przewoźnik nie obsługuje usługi „Uwaga, ostrożnie”")
}

// DefaultInput prepares a shipment from the configuration and the delivery the customer chose.
func (s *Service) DefaultInput(ctx context.Context, cfg Config, orderID string, destination Destination, goods []Good, insurance domain.Money) ShipmentInput {
	opt, _ := cfg.Option(destination.Method)
	pack := PackFor(cfg, goods)
	in := ShipmentInput{
		OrderID: orderID, Carrier: opt.Carrier, LengthCm: pack.Length, WidthCm: pack.Width, HeightCm: pack.Height, WeightKg: pack.Weight,
		Description: cfg.Parcel.Description, DropOff: opt.NeedsLocker(),
		Recipient: polkurier.Address{Person: destination.Person, Email: destination.Email, Phone: destination.Phone, Country: "PL"},
	}
	if cfg.Parcel.Insure {
		in.Insurance = insurance.Zloty()
	}
	if destination.PointID != "" {
		in.Recipient.PointID = destination.PointID
		in.Recipient.Street, in.Recipient.HouseNumber, in.Recipient.FlatNumber = SplitStreet(destination.Street)
		in.Recipient.Postcode, in.Recipient.City = destination.Postcode, destination.City
		if opt.Carrier != "" && s.Configured() && (destination.Street == "" || destination.Postcode == "") {
			if pt, err := s.LookupPoint(ctx, opt.Carrier, destination.PointID); err == nil {
				in.Recipient.Street, in.Recipient.HouseNumber, in.Recipient.FlatNumber = SplitStreet(pt.Street)
				in.Recipient.Postcode, in.Recipient.City = pt.Postcode, pt.City
			}
		}
	} else {
		in.Recipient.Street, in.Recipient.HouseNumber, in.Recipient.FlatNumber = SplitStreet(destination.Street)
		in.Recipient.Postcode, in.Recipient.City = destination.Postcode, destination.City
	}
	if in.Recipient.HouseNumber == "" {
		in.Recipient.HouseNumber = "1"
	}
	return in
}

// Destination is where the customer wants the order delivered (see domain.Delivery).
type Destination struct {
	Method   string
	Person   string
	Email    string
	Phone    string
	Street   string
	Postcode string
	City     string
	PointID  string
}

// SenderAddress builds the sender address from the configuration.
func SenderAddress(sender Sender) (*polkurier.Address, error) {
	if missing := sender.Missing(); missing != "" {
		return nil, errors.New(missing)
	}
	return &polkurier.Address{
		Company: sender.Company, Person: sender.Person, Street: sender.Street, HouseNumber: sender.HouseNumber, FlatNumber: sender.FlatNumber,
		Postcode: sender.Postcode, City: sender.City, Email: sender.Email, Phone: sender.Phone, Country: "PL",
	}, nil
}

// CreateShipment orders the parcel at Polkurier and stores it.
func (s *Service) CreateShipment(ctx context.Context, cfg Config, in ShipmentInput) (*Shipment, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	sender, err := SenderAddress(cfg.Sender)
	if err != nil {
		return nil, err
	}
	recipient := in.Recipient
	req := polkurier.OrderRequest{
		ShipmentType: polkurier.ShipmentBox, Courier: in.Carrier, Description: in.Description,
		Sender: sender, Recipient: &recipient,
		Packs:  []polkurier.Pack{{Length: in.LengthCm, Width: in.WidthCm, Height: in.HeightCm, Weight: in.WeightKg, Amount: 1, Type: polkurier.PackStandard}},
		Pickup: &polkurier.Pickup{NoCourierOrder: in.DropOff, Date: polkurier.Date(in.PickupDate), TimeFrom: in.PickupFrom, TimeTo: in.PickupTo},
	}
	if in.Insurance > 0 {
		req.Insurance = in.Insurance
	}
	if req.Services, err = s.selectedServices(ctx, in.Carrier, in.HandleWithCare); err != nil {
		return nil, err
	}
	created, err := client.CreateOrder(ctx, req)
	if err != nil {
		return nil, err
	}
	waybill := ""
	if len(created.Waybills) > 0 {
		waybill = created.Waybills[0]
	}
	sh, err := s.Store.Insert(ctx, Shipment{
		OrderID: in.OrderID, Carrier: in.Carrier, ExternalNumber: created.OrderNumber, Waybill: waybill, TrackingURL: created.TrackingURL,
		PriceGross: created.PriceGross.Grosz(), Status: "Utworzona", StatusCode: string(polkurier.StatusConfirmed),
	})
	if err != nil {
		return nil, fmt.Errorf("przesyłka %s została utworzona w Polkurierze, ale nie zapisała się w sklepie: %w", created.OrderNumber, err)
	}
	return sh, nil
}

// Label returns the PDF label of a parcel.
func (s *Service) Label(ctx context.Context, sh *Shipment) ([]byte, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	return client.GetLabel(ctx, sh.ExternalNumber)
}

// Cancel cancels a parcel that has not been picked up yet.
func (s *Service) Cancel(ctx context.Context, sh *Shipment) error {
	client := s.api()
	if client == nil {
		return ErrNotConfigured
	}
	if _, err := client.CancelOrder(ctx, sh.ExternalNumber); err != nil {
		return err
	}
	return s.Store.Cancel(ctx, sh.ID)
}

// Refresh asks Polkurier for the current status of a parcel, stores it and returns it.
func (s *Service) Refresh(ctx context.Context, sh *Shipment) (*Shipment, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	st, err := client.GetStatus(ctx, sh.ExternalNumber)
	if err != nil {
		return nil, err
	}
	if err := s.Store.UpdateStatus(ctx, sh.ID, st.Status, string(st.StatusCode), st.URL); err != nil {
		return nil, err
	}
	return s.Store.ByID(ctx, sh.ID)
}

// InTransit reports whether the courier has the parcel (or already delivered it).
func InTransit(sh *Shipment) bool {
	return sh.StatusCode == string(polkurier.StatusInTransit) || sh.StatusCode == string(polkurier.StatusDelivered)
}

// Problem reports a delivery problem or a return.
func Problem(sh *Shipment) bool {
	return sh.StatusCode == string(polkurier.StatusException) || sh.StatusCode == string(polkurier.StatusReturned)
}

// RunSync refreshes the parcels that are still on their way every interval and
// calls changed with each parcel whose status changed.
func (s *Service) RunSync(ctx context.Context, every time.Duration, changed func(ctx context.Context, before, after *Shipment)) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncOnce(ctx, changed)
		}
	}
}

// SyncOnce refreshes every parcel that is still on its way.
func (s *Service) SyncOnce(ctx context.Context, changed func(ctx context.Context, before, after *Shipment)) {
	if !s.Configured() {
		return
	}
	list, err := s.Store.Trackable(ctx)
	if err != nil {
		s.logger().Error("shipments to track", "err", err)
		return
	}
	for _, sh := range list {
		after, err := s.Refresh(ctx, sh)
		if err != nil {
			s.logger().Warn("shipment status", "shipment", sh.ExternalNumber, "err", err)
			continue
		}
		if after.StatusCode != sh.StatusCode && changed != nil {
			changed(ctx, sh, after)
		}
	}
}

// PickupDays returns courier pickup windows for the next working days.
func (s *Service) PickupDays(ctx context.Context, carrier string, sender Sender) ([]polkurier.PickupDay, error) {
	client := s.api()
	if client == nil {
		return nil, ErrNotConfigured
	}
	return client.GetCourierPickupTime(ctx, carrier, sender.Postcode, "", polkurier.ShipmentBox)
}
