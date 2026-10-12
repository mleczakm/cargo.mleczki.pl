package domain_test

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"cargo.mleczki.pl/internal/domain"
)

func TestParsePLN(t *testing.T) {
	valid := map[string]int64{
		"150": 15000, "49.90": 4990, "49,9": 4990, "0.05": 5, ".5": 50, " 1 234,50 zł ": 123450,
		"0": 0, "-3.20": -320, "7.": 700, "12 000": 1200000,
	}
	for in, want := range valid {
		got, err := domain.ParsePLN(in)
		if err != nil || got.Minor() != want {
			t.Errorf("ParsePLN(%q) = %d, %v; want %d", in, got.Minor(), err, want)
		}
	}
	for _, in := range []string{"", "abc", "1.234", "12.5.6", "1e3", "--1", "9999999999999999"} {
		if got, err := domain.ParsePLN(in); err == nil {
			t.Errorf("ParsePLN(%q) = %d, want an error", in, got.Minor())
		}
	}
}

func TestMoneyArithmeticIsExact(t *testing.T) {
	price := domain.PLN(1999) // 19.99
	if got := price.Mul(3); got.Minor() != 5997 {
		t.Errorf("3 × 19.99 = %d", got.Minor())
	}
	// The classic float trap: 0.1 + 0.2.
	if got := domain.PLN(10).Add(domain.PLN(20)); got.Minor() != 30 {
		t.Errorf("0.10 + 0.20 = %d", got.Minor())
	}
	if got := domain.SumMoney(domain.Zloty(150), domain.PLN(5), domain.PLN(5)); got != domain.PLN(15010) {
		t.Errorf("sum = %v", got)
	}
	if got := domain.Zloty(10).Sub(domain.PLN(1)); got != domain.PLN(999) {
		t.Errorf("10 - 0.01 = %v", got)
	}
	if !domain.PLN(0).IsZero() || !(domain.Money{}).IsZero() || !domain.PLN(1).IsPositive() || !domain.PLN(-1).IsNegative() {
		t.Error("sign helpers")
	}
	if !domain.PLN(1).Less(domain.PLN(2)) || domain.PLN(2).Less(domain.PLN(2)) {
		t.Error("Less")
	}
}

func TestMoneyDisplay(t *testing.T) {
	cases := map[int64]string{
		0: "0 zł", 15000: "150 zł", 4990: "49,90 zł", 5: "0,05 zł", 123450: "1 234,50 zł",
		123400: "1 234 zł", -320: "-3,20 zł",
	}
	for minor, want := range cases {
		if got := domain.PLN(minor).String(); got != want {
			t.Errorf("PLN(%d).String() = %q, want %q", minor, got, want)
		}
	}
	if got := domain.PLN(4990).Plain(); got != "49.90" {
		t.Errorf("Plain = %q", got)
	}
	if got := domain.PLN(-5).Plain(); got != "-0.05" {
		t.Errorf("Plain = %q", got)
	}
}

func TestMoneyJSONRoundTripsAndReadsLegacyNumbers(t *testing.T) {
	type payload struct {
		Price domain.Money `json:"price"`
	}
	out, err := json.Marshal(payload{Price: domain.PLN(4990)})
	if err != nil || string(out) != `{"price":"49.90"}` {
		t.Fatalf("marshal: %s %v", out, err)
	}

	for in, want := range map[string]int64{
		`{"price":"49.90"}`: 4990,
		`{"price":150}`:     15000, // stored before decimals existed: whole złoty
		`{"price":12.5}`:    1250,
		`{"price":null}`:    0,
		`{}`:                0,
	} {
		var got payload
		if err := json.Unmarshal([]byte(in), &got); err != nil || got.Price.Minor() != want {
			t.Errorf("unmarshal %s = %d, %v; want %d", in, got.Price.Minor(), err, want)
		}
	}
	var bad payload
	if err := json.Unmarshal([]byte(`{"price":"1.234"}`), &bad); err == nil {
		t.Error("three decimals must be rejected")
	}
}

func TestMoneyYAMLIsExact(t *testing.T) {
	var doc struct {
		A domain.Money `yaml:"a"`
		B domain.Money `yaml:"b"`
		C domain.Money `yaml:"c"`
	}
	if err := yaml.Unmarshal([]byte("a: 150\nb: 49.99\nc: '0,5'\n"), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.A.Minor() != 15000 || doc.B.Minor() != 4999 || doc.C.Minor() != 50 {
		t.Errorf("got %v %v %v", doc.A.Minor(), doc.B.Minor(), doc.C.Minor())
	}
	if err := yaml.Unmarshal([]byte("a: 1.999\n"), &doc); err == nil {
		t.Error("too many decimals must be rejected")
	}
	if err := yaml.Unmarshal([]byte("a: [1]\n"), &doc); err == nil {
		t.Error("a list is not a price")
	}
}

func TestMoneyZloty(t *testing.T) {
	if got := domain.MoneyFromZloty(49.9); got != domain.PLN(4990) {
		t.Errorf("49.9 = %d", got.Minor())
	}
	if got := domain.MoneyFromZloty(0.1 + 0.2); got != domain.PLN(30) {
		t.Errorf("float noise must round away, got %d", got.Minor())
	}
	if got := domain.PLN(4990).Zloty(); got != 49.9 {
		t.Errorf("Zloty = %v", got)
	}
}
