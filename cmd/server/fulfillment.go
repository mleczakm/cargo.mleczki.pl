package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/shipping"
)

// fulfillmentSettings are the payment methods offered at checkout and the details
// shown to customers. Delivery options live in data/shipping.yml (see internal/shipping).
type fulfillmentSettings struct {
	BankTransfer  bool   `json:"bankTransfer"`
	PickupPayment bool   `json:"pickupPayment"`
	Blik          bool   `json:"blik"`
	BankAccount   string `json:"bankAccount"`
	BankRecipient string `json:"bankRecipient"`
	BlikPhone     string `json:"blikPhone"`
}

func defaultFulfillmentSettings() fulfillmentSettings {
	return fulfillmentSettings{
		BankTransfer:  paymentEnabled("PAYMENT_BANK_TRANSFER_ENABLED", true),
		PickupPayment: paymentEnabled("PAYMENT_PICKUP_ENABLED", true),
		Blik:          paymentEnabled("PAYMENT_BLIK_ENABLED", false),
		BankAccount:   strings.TrimSpace(getenvOr("PAYMENT_BANK_ACCOUNT", "91 2490 0005 0000 4000 9728 6978")),
		BankRecipient: getenvOr("PAYMENT_BANK_RECIPIENT", "Michał Mleczko"),
		BlikPhone:     "732 575 375",
	}
}

func getenvOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
func (s *Server) fulfillmentSettings() (fulfillmentSettings, error) {
	st := defaultFulfillmentSettings()
	var raw string
	err := s.readModels.GetDB().QueryRow(`SELECT value FROM business_settings WHERE key = 'fulfillment'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	err = json.Unmarshal([]byte(raw), &st)
	return st, err
}
func (s *Server) saveFulfillmentSettings(st fulfillmentSettings) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.readModels.GetDB().Exec(`INSERT INTO business_settings(key,value) VALUES ('fulfillment',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	return err
}

// checkoutMethods lists every payment method usable with at least one delivery
// option, tagged with the options it is valid for. Without delivery options (a
// rental-only cart) everything is collected in person.
func (st fulfillmentSettings) checkoutMethods(options []deliveryOptionView) []checkoutPaymentMethod {
	if len(options) == 0 {
		options = []deliveryOptionView{{Option: shipping.Option{Code: shipping.PickupCode, Kind: shipping.KindPickup}}}
	}
	var out []checkoutPaymentMethod
	index := map[string]int{}
	for _, opt := range options {
		for _, method := range st.methods(opt.Kind != shipping.KindPickup) {
			i, ok := index[method.ID]
			if !ok {
				method.Selected = false
				out = append(out, method)
				i = len(out) - 1
				index[method.ID] = i
			}
			out[i].Shippings = append(out[i].Shippings, opt.Code)
		}
	}
	for i := range out {
		for _, code := range out[i].Shippings {
			if code == options[0].Code {
				out[i].Selected = true
				return out
			}
		}
	}
	return out
}

// methods are the enabled payment methods for an order that is shipped or collected.
// A parcel leaves only once it is paid, so shipped orders get the prepaid methods.
func (st fulfillmentSettings) methods(shipped bool) []checkoutPaymentMethod {
	var methods []checkoutPaymentMethod
	if st.BankTransfer {
		methods = append(methods, checkoutPaymentMethod{ID: domain.PaymentMethodBankTransfer, Label: "Przelew tradycyjny", Description: "Przelew na rachunek bankowy."})
	}
	if st.Blik {
		methods = append(methods, checkoutPaymentMethod{ID: domain.PaymentMethodBlik, Label: "Przelew BLIK na telefon", Description: "Płatność BLIK na telefon."})
	}
	if st.PickupPayment && !shipped {
		methods = append(methods, checkoutPaymentMethod{ID: domain.PaymentMethodCashPickup, Label: "Płatność przy odbiorze", Description: "Płatność na miejscu w Radzyminie."})
	}
	if len(methods) > 0 {
		methods[0].Selected = true
	}
	return methods
}

// allows reports whether the method is enabled for this kind of delivery.
func (st fulfillmentSettings) allows(method string, shipped bool) bool {
	for _, m := range st.methods(shipped) {
		if m.ID == method {
			return true
		}
	}
	return false
}

func (s *Server) handleAdminFulfillmentSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.fulfillmentSettings()
	if err != nil {
		http.Error(w, "Nie można odczytać ustawień", 500)
		return
	}
	st.BankTransfer = r.FormValue("bank_transfer") == "on"
	st.PickupPayment = r.FormValue("pickup_payment") == "on"
	st.Blik = r.FormValue("blik") == "on"
	st.BankAccount = strings.TrimSpace(r.FormValue("bank_account"))
	st.BankRecipient = strings.TrimSpace(r.FormValue("bank_recipient"))
	st.BlikPhone = strings.TrimSpace(r.FormValue("blik_phone"))
	if st.BankTransfer && st.BankAccount == "" {
		s.adminBack(w, r, "fulfillment", "error", "Podaj numer rachunku dla przelewu.")
		return
	}
	if st.Blik && st.BlikPhone == "" {
		s.adminBack(w, r, "fulfillment", "error", "Podaj numer telefonu BLIK.")
		return
	}
	if !st.BankTransfer && !st.Blik && !st.PickupPayment {
		s.adminBack(w, r, "fulfillment", "error", "Włącz przynajmniej jedną metodę płatności.")
		return
	}
	if err := s.saveFulfillmentSettings(st); err != nil {
		http.Error(w, "Nie można zapisać ustawień", 500)
		return
	}
	s.adminBack(w, r, "fulfillment", "notice", "Zapisano ustawienia płatności.")
}

// paymentAllowed checks the method against the current settings. An unreadable
// settings store fails closed.
func (s *Server) paymentAllowed(method string, shipped bool) bool {
	st, err := s.fulfillmentSettings()
	return err == nil && st.allows(method, shipped)
}
