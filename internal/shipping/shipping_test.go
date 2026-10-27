//nolint:testpackage // These tests cover internal carrier and store behavior.
package shipping

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	polkurier "github.com/mleczakm/polkurier-go"
	_ "modernc.org/sqlite"

	"cargo.mleczki.pl/internal/domain"
)

func TestSplitStreet(t *testing.T) {
	cases := map[string][3]string{
		"Poznańska 12":            {"Poznańska", "12", ""},
		"ul. Poznańska 12/3":      {"Poznańska", "12", "3"},
		"Aleja Jana Pawła II 12D": {"Aleja Jana Pawła II", "12D", ""},
		"3 Maja 5 m. 7":           {"3 Maja", "5", "7"},
		"Rynek":                   {"Rynek", "", ""},
	}
	for in, want := range cases {
		s, h, f := SplitStreet(in)
		if s != want[0] || h != want[1] || f != want[2] {
			t.Errorf("SplitStreet(%q) = %q %q %q, want %v", in, s, h, f, want)
		}
	}
}

func TestSplitPostcodeCity(t *testing.T) {
	code, city := SplitPostcodeCity("05-250 Radzymin")
	if code != "05-250" || city != "Radzymin" {
		t.Fatalf("got %q %q", code, city)
	}
	if code, city = SplitPostcodeCity("Radzymin"); code != "" || city != "Radzymin" {
		t.Fatalf("got %q %q", code, city)
	}
	if !ValidPostcode("05-250") || ValidPostcode("05250") || ValidPostcode("05-250 x") {
		t.Fatal("postcode validation is wrong")
	}
}

const sampleConfig = `
sender:
  person: Michał Mleczko
  street: Przykładowa
  houseNumber: "1"
  postcode: 05-250
  city: Radzymin
  email: a@example.com
  phone: "500100200"
parcel:
  lengthCm: 40
  widthCm: 30
  heightCm: 15
options:
  - code: pickup
    name: Odbiór osobisty
  - code: inpost
    name: Paczkomat
    kind: locker
    carrier: inpost_paczkomat
    price: 14.99
    freeFrom: 200
  - code: kurier
    name: Kurier
    kind: courier
    carrier: DPD
    price: "19,90"
`

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(sampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	locker, ok := cfg.Option("inpost")
	if !ok || locker.Carrier != "INPOST_PACZKOMAT" || !locker.NeedsLocker() || !locker.NeedsPolkurier() {
		t.Fatalf("locker = %+v", locker)
	}
	if got := locker.Cost(domain.Zloty(100)); got != domain.PLN(1499) {
		t.Errorf("cost below the free threshold = %v", got)
	}
	if got := locker.Cost(domain.Zloty(200)); !got.IsZero() {
		t.Errorf("cost at the free threshold = %v, want free", got)
	}
	courier, _ := cfg.Option("kurier")
	if courier.Price != domain.PLN(1990) || !courier.NeedsAddress() {
		t.Errorf("courier = %+v", courier)
	}
	if cfg.Parcel.PackagingWeightG != 300 || cfg.Parcel.Description == "" {
		t.Errorf("parcel defaults were not filled: %+v", cfg.Parcel)
	}
	if len(cfg.Available(false)) != 1 || len(cfg.Available(true)) != 3 {
		t.Errorf("options without Polkurier = %d, with = %d", len(cfg.Available(false)), len(cfg.Available(true)))
	}
	if cfg.Sender.Missing() != "" {
		t.Errorf("sender is complete but Missing() = %q", cfg.Sender.Missing())
	}
}

func TestParseConfigAddsPickupAndRejectsMistakes(t *testing.T) {
	cfg, err := ParseConfig([]byte("options:\n  - {code: kurier, name: Kurier, kind: courier, carrier: DPD, price: 20}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Option(PickupCode); !ok || cfg.Options[0].Code != PickupCode {
		t.Errorf("pickup was not added first: %+v", cfg.Options)
	}

	for name, doc := range map[string]string{
		"no carrier":     "options:\n  - {code: kurier, name: Kurier, kind: courier}\n",
		"bad kind":       "options:\n  - {code: x, name: X, kind: drone, carrier: A}\n",
		"repeated code":  "options:\n  - {code: x, name: X}\n  - {code: x, name: Y}\n",
		"bad code":       "options:\n  - {code: 'Za Duże', name: X}\n",
		"negative price": "options:\n  - {code: x, name: X, price: -1}\n",
		"three decimals": "options:\n  - {code: x, name: X, price: 1.234}\n",
		"missing name":   "options:\n  - {code: x}\n",
		"not even yaml":  "options: [",
	} {
		if _, err := ParseConfig([]byte(doc)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestConfigFileFallsBackAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shipping.yml")
	configFile := &ConfigFile{Path: path}

	cfg, err := configFile.Load()
	if err != nil || len(cfg.Options) != 1 || cfg.Options[0].Code != PickupCode {
		t.Fatalf("missing file should mean pickup only: %+v, %v", cfg, err)
	}

	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err = configFile.Load(); err != nil || len(cfg.Options) != 3 {
		t.Fatalf("after writing: %d options, %v", len(cfg.Options), err)
	}

	// A broken edit keeps the last good configuration serving and reports the problem.
	if err := os.WriteFile(path, []byte("options: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(path, future, future)
	cfg, err = configFile.Load()
	if err == nil || len(cfg.Options) != 3 {
		t.Fatalf("broken file: %d options, err %v", len(cfg.Options), err)
	}
}

func TestPackForSumsWeightAndGrowsTheBox(t *testing.T) {
	cfg := DefaultConfig()
	pack := PackFor(cfg, []Good{{Quantity: 2, WeightG: 450}, {Quantity: 1, LengthCm: 60, HeightCm: 12}})
	// 300 packaging + 2*450 + 500 default = 1700 g
	if pack.Weight != 1.7 {
		t.Errorf("weight = %v, want 1.7", pack.Weight)
	}
	if pack.Length != 60 || pack.Width != 20 || pack.Height != 12 {
		t.Errorf("box = %dx%dx%d", pack.Length, pack.Width, pack.Height)
	}
	if empty := PackFor(cfg, nil); empty.Weight != 0.3 {
		t.Errorf("empty parcel weighs %v", empty.Weight)
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(Schema); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func TestStoreLifecycle(t *testing.T) {
	ctx := t.Context()
	store := newStore(t)

	sh, err := store.Insert(ctx, Shipment{OrderID: "ORD-1", Carrier: "DPD", ExternalNumber: "PK1", Waybill: "W1", PriceGross: 1234, Status: "Utworzona", StatusCode: "P"})
	if err != nil {
		t.Fatal(err)
	}
	if !sh.Active() || !sh.Tracked() || sh.Delivered() {
		t.Fatalf("fresh shipment: %+v", sh)
	}
	if list, _ := store.Trackable(ctx); len(list) != 1 {
		t.Fatalf("trackable = %d", len(list))
	}

	if err := store.UpdateStatus(ctx, sh.ID, "Doręczona", "D", "https://track/1"); err != nil {
		t.Fatal(err)
	}
	sh, _ = store.ByID(ctx, sh.ID)
	if !sh.Delivered() || sh.Tracked() || sh.TrackingURL != "https://track/1" {
		t.Fatalf("delivered shipment: %+v", sh)
	}
	// An update without a tracking URL keeps the one we have.
	_ = store.UpdateStatus(ctx, sh.ID, "Doręczona", "D", "")
	if sh, _ = store.ByID(ctx, sh.ID); sh.TrackingURL != "https://track/1" {
		t.Errorf("tracking url was lost: %q", sh.TrackingURL)
	}

	second, _ := store.Insert(ctx, Shipment{OrderID: "ORD-1", Carrier: "DPD", ExternalNumber: "PK2"})
	if err := store.Cancel(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if second, _ = store.ByID(ctx, second.ID); second.Active() || second.Tracked() {
		t.Errorf("cancelled shipment is still active: %+v", second)
	}
	list, _ := store.ForOrder(ctx, "ORD-1")
	if len(list) != 2 || list[0].ExternalNumber != "PK2" {
		t.Errorf("for order = %+v", list)
	}
	if _, err := store.ByID(ctx, 999); !errors.Is(err, ErrShipmentNotFound) {
		t.Errorf("unknown id error = %v", err)
	}
}

// fakeAPI answers the Polkurier methods the service uses.
func fakeAPI(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		switch {
		case strings.Contains(body, `"apimethod":"available_carriers"`):
			*calls = append(*calls, "carriers")
			_, _ = io.WriteString(w, `{"status":"success","response":[{"servicecode":"DPD","name":"DPD Classic"}]}`)
		case strings.Contains(body, `"apimethod":"order_valuation_v2"`):
			*calls = append(*calls, "quote")
			_, _ = io.WriteString(w, `{"status":"success","response":[
				{"servicecode":"DPD","serviceName":"DPD","grossprice":24.60,"available":true},
				{"servicecode":"INPOST_PACZKOMAT","serviceName":"InPost","grossprice":12.30,"available":true}]}`)
		case strings.Contains(body, `"apimethod":"get_courier_point"`):
			*calls = append(*calls, "points")
			_, _ = io.WriteString(w, `{"status":"success","response":[
				{"id":"RDZ01M","city":"Radzymin","zip":"05-250","street":"Konstytucji 3 Maja 2","available":true},
				{"id":"RDZ02M","city":"Radzymin","zip":"05-250","street":"Nieczynna 1","available":false}]}`)
		case strings.Contains(body, `"apimethod":"create_order"`):
			*calls = append(*calls, "create")
			_, _ = io.WriteString(w, `{"status":"success","response":{"order_number":"PK-77","label":["WB123"],"price_gross":"11.50","url_tracktrace":"https://track/77","is_paid":true}}`)
		case strings.Contains(body, `"apimethod":"get_status"`):
			*calls = append(*calls, "status")
			_, _ = io.WriteString(w, `{"status":"success","response":{"status":"W drodze","status_code":"WP","url":"https://track/77"}}`)
		case strings.Contains(body, `"apimethod":"cancel_order"`):
			*calls = append(*calls, "cancel")
			_, _ = io.WriteString(w, `{"status":"success","response":{"cancellation":true}}`)
		default:
			t.Errorf("unexpected API request: %s", body)
			_, _ = io.WriteString(w, `{"status":"error","response":"unexpected"}`)
		}
	}))
}

func TestServiceWithoutCredentialsIsOff(t *testing.T) {
	service := &Service{}
	service.Configure(" ", "token", true, "", "test")
	if service.Configured() {
		t.Fatal("a missing login must leave Polkurier off")
	}
	if _, err := service.SearchPoints(t.Context(), "X", "Radzymin"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("SearchPoints error = %v", err)
	}
	if _, err := service.CreateShipment(t.Context(), DefaultConfig(), ShipmentInput{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("CreateShipment error = %v", err)
	}
	service.SyncOnce(t.Context(), nil) // must not panic or call anything
}

func TestServiceQuotesFindsPointsAndShips(t *testing.T) {
	var calls []string
	api := fakeAPI(t, &calls)
	defer api.Close()

	ctx := t.Context()
	service := &Service{Store: newStore(t)}
	service.Configure("acct", "tok", true, api.URL, "test")
	cfg, _ := ParseConfig([]byte(sampleConfig))

	quotes, err := service.Quote(ctx, cfg.Sender, PackFor(cfg, nil), "00-001", 0)
	if err != nil || len(quotes) != 2 || quotes[0].ServiceCode != "INPOST_PACZKOMAT" {
		t.Fatalf("quotes sorted by price: %+v, %v", quotes, err)
	}
	if _, err := service.Quote(ctx, cfg.Sender, PackFor(cfg, nil), "00-001", 0); err != nil {
		t.Fatal(err)
	}
	if n := count(calls, "quote"); n != 1 {
		t.Errorf("quote asked %d times, want 1 (cached)", n)
	}
	if v, ok := PriceFor(quotes, "dpd"); !ok || v.FinalGross().Grosz() != 2460 {
		t.Errorf("PriceFor = %+v %v", v, ok)
	}

	points, err := service.SearchPoints(ctx, "INPOST_PACZKOMAT", "Radzymin")
	if err != nil || len(points) != 1 || points[0].ID != "RDZ01M" {
		t.Fatalf("unavailable points must be left out: %+v, %v", points, err)
	}
	if pt, err := service.LookupPoint(ctx, "INPOST_PACZKOMAT", "rdz01m"); err != nil || pt.Postcode != "05-250" {
		t.Errorf("LookupPoint = %+v, %v", pt, err)
	}
	if _, err := service.LookupPoint(ctx, "INPOST_PACZKOMAT", "ZZZ99"); !errors.Is(err, ErrPointNotFound) {
		t.Errorf("unknown locker error = %v", err)
	}

	in := service.DefaultInput(ctx, cfg, "ORD-9", Destination{Method: "inpost", Person: "Jan Kowalski", Email: "j@example.com", Phone: "600", PointID: "RDZ01M"},
		[]Good{{Quantity: 1, WeightG: 700}}, domain.Zloty(80))
	if in.Carrier != "INPOST_PACZKOMAT" || !in.DropOff || in.Recipient.PointID != "RDZ01M" || in.Recipient.Postcode != "05-250" || in.Recipient.Street != "Konstytucji 3 Maja" {
		t.Fatalf("default input = %+v", in)
	}
	if in.WeightKg != 1.0 {
		t.Errorf("weight = %v", in.WeightKg)
	}

	sh, err := service.CreateShipment(ctx, cfg, in)
	if err != nil || sh.ExternalNumber != "PK-77" || sh.Waybill != "WB123" || sh.PriceGross != 1150 {
		t.Fatalf("shipment = %+v, %v", sh, err)
	}

	var changed []string
	service.SyncOnce(ctx, func(_ context.Context, before, after *Shipment) {
		changed = append(changed, before.StatusCode+">"+after.StatusCode)
	})
	if len(changed) != 1 || changed[0] != "P>WP" {
		t.Errorf("status changes = %v", changed)
	}
	if got, _ := service.Store.ByID(ctx, sh.ID); !InTransit(got) || Problem(got) {
		t.Errorf("after sync: %+v", got)
	}

	if err := service.Cancel(ctx, sh); err != nil {
		t.Fatal(err)
	}
	if got, _ := service.Store.ByID(ctx, sh.ID); got.Active() {
		t.Errorf("cancelled shipment is active: %+v", got)
	}
}

func TestCreateShipmentValidatesBeforeCallingTheAPI(t *testing.T) {
	var calls []string
	api := fakeAPI(t, &calls)
	defer api.Close()
	service := &Service{Store: newStore(t)}
	service.Configure("acct", "tok", true, api.URL, "test")
	cfg, _ := ParseConfig([]byte(sampleConfig))

	good := ShipmentInput{OrderID: "ORD-1", Carrier: "DPD", LengthCm: 10, WidthCm: 10, HeightCm: 10, WeightKg: 1,
		Recipient: polkurier.Address{Person: "A B", Email: "a@b.pl", Phone: "1", Street: "S", HouseNumber: "1", Postcode: "00-001", City: "C"}}

	bad := map[string]func(in *ShipmentInput){
		"no carrier": func(in *ShipmentInput) { in.Carrier = "" },
		"no weight":  func(in *ShipmentInput) { in.WeightKg = 0 },
		"no phone":   func(in *ShipmentInput) { in.Recipient.Phone = "" },
		"no address": func(in *ShipmentInput) { in.Recipient.Street = "" },
		"no order":   func(in *ShipmentInput) { in.OrderID = "" },
	}
	for name, mutate := range bad {
		in := good
		mutate(&in)
		if _, err := service.CreateShipment(t.Context(), cfg, in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	noSender := cfg
	noSender.Sender = Sender{}
	if _, err := service.CreateShipment(t.Context(), noSender, good); err == nil || !strings.Contains(err.Error(), "nadawc") {
		t.Errorf("missing sender error = %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("the API was called for invalid input: %v", calls)
	}
}

func count(list []string, what string) int {
	count2 := 0
	for _, s := range list {
		if s == what {
			count2++
		}
	}
	return count2
}
