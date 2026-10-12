package main

import (
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"cargo.mleczki.pl/internal/domain"
)

const (
	dateLayout = "2006-01-02"
	// maxRentalDays bounds a single rental line. Every day of a rental is a row
	// in the bookings table and a check at checkout, so an unbounded range (a
	// forged cookie, a typo in the year) would be a cheap way to hurt the server.
	maxRentalDays = 90
)

// today is the first day a rental may start. The calendar widget works in UTC too.
func today() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }

func parseRentalDates(startDate, endDate string) (time.Time, time.Time, error) {
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start: %w", err)
	}
	end, err := time.Parse(dateLayout, endDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end: %w", err)
	}
	return start, end, nil
}

func rentalDaysBetween(start, end time.Time) int {
	return int(end.Sub(start)/(24*time.Hour)) + 1
}

// priceRentalLine fills a rental line from the product: name, base price, the
// number of days (from the dates), the add-ons and the total. Only the product,
// the dates and the add-on IDs of the incoming line are used. A non-empty second
// result says why the line cannot be ordered as it is; the line is still filled
// as far as possible so it can be shown with the explanation.
func priceRentalLine(product domain.Product, line CartItem, now time.Time) (CartItem, string) {
	line.Kind = ""
	line.ProductName = product.Name
	line.BasePrice = product.BasePrice
	line.Quantity = 0
	line.SKU = ""

	start, end, err := parseRentalDates(line.StartDate, line.EndDate)
	if err != nil {
		line.RentalDays, line.Total = 0, domain.Money{}
		return line, "Wybierz poprawny termin wynajmu."
	}
	line.RentalDays = rentalDaysBetween(start, end)
	if end.Before(start) {
		line.RentalDays = 0
	}
	addonIDs := make([]string, 0, len(line.Addons))
	for _, id := range line.Addons {
		if !slices.Contains(addonIDs, id) {
			addonIDs = append(addonIDs, id)
		}
	}
	line.Addons = addonIDs
	line.Total = domain.RentalPrice(product.BasePrice, productAddons(product, addonIDs), line.RentalDays)

	switch {
	case end.Before(start):
		return line, "Data zwrotu nie może być przed datą odbioru."
	case start.Before(now):
		return line, "Termin wynajmu jest już w przeszłości."
	case line.RentalDays > maxRentalDays:
		return line, fmt.Sprintf("Jednorazowo można wynająć sprzęt na maksymalnie %d dni.", maxRentalDays)
	}
	for _, id := range addonIDs {
		if len(productAddons(product, []string{id})) == 0 {
			return line, "Wybrany dodatek nie jest już dostępny."
		}
	}
	return line, ""
}

// rentalSpan is the period covered by the rental lines of a cart: the earliest
// start, the latest end, and the number of days in between. Orders also carry
// the dates of every line, so this is only the order-level summary.
func rentalSpan(cart []CartItem) (string, string, int) {
	var first, last time.Time
	for _, item := range cart {
		if item.IsShop() {
			continue
		}
		start, end, err := parseRentalDates(item.StartDate, item.EndDate)
		if err != nil {
			continue
		}
		if first.IsZero() || start.Before(first) {
			first = start
		}
		if last.IsZero() || end.After(last) {
			last = end
		}
	}
	if first.IsZero() {
		return "", "", 0
	}
	return first.Format(dateLayout), last.Format(dateLayout), rentalDaysBetween(first, last)
}

// normalizeCart prices and validates every line of a cart against the catalog,
// the calendar and the stock ledger. It returns the cart to use from now on
// and, per CartID, what is wrong with a line.
func (s *Server) normalizeCart(r *http.Request, cart []CartItem) ([]CartItem, map[string]string) {
	problems := map[string]string{}
	if len(cart) == 0 {
		return cart, problems
	}
	out := slices.Clone(cart)
	s.normalizeRentalLines(r, out, problems)
	s.normalizeShopLines(r, out, problems)
	return out, problems
}

type bookedSpan struct{ start, end time.Time }

func (s *Server) normalizeRentalLines(r *http.Request, out []CartItem, problems map[string]string) {
	if !cartHasRentalLines(out) {
		return
	}

	catalog := map[string]domain.Product{}
	products, err := s.productParser.LoadAllProducts()
	if err != nil {
		log.Printf("Failed to load products for cart: %v", err)
	}
	for _, product := range filterProductsByVisibility(products, r) {
		catalog[product.ID] = product
	}
	closed, err := s.readModels.GetGlobalBlockedDates()
	if err != nil {
		log.Printf("Failed to load store closures for cart: %v", err)
	}

	taken := map[string][]bookedSpan{} // earlier lines of this cart, per product
	now := today()
	for i, line := range out {
		if line.IsShop() {
			continue
		}
		product, ok := catalog[line.ProductID]
		if !ok {
			problems[line.CartID] = "Ten sprzęt nie jest już dostępny w ofercie."
			continue
		}
		priced, problem := priceRentalLine(product, line, now)
		out[i] = priced
		if problem == "" {
			problem = s.rentalConflict(product, priced, closed, taken)
		}
		if problem != "" {
			problems[line.CartID] = problem
		}
	}
}

// rentalConflict says why the dates of a (valid) rental line cannot be had:
// another line of the same cart overlaps, or the days are closed or booked.
func (s *Server) rentalConflict(product domain.Product, line CartItem, closed []string, taken map[string][]bookedSpan) string {
	start, end, err := parseRentalDates(line.StartDate, line.EndDate)
	if err != nil {
		return "Wybierz poprawny termin wynajmu."
	}
	for _, span := range taken[product.ID] {
		if !end.Before(span.start) && !start.After(span.end) {
			return "Ten sprzęt jest już w koszyku w nakładającym się terminie."
		}
	}
	taken[product.ID] = append(taken[product.ID], bookedSpan{start, end})

	dbBooked, err := s.readModels.GetBookedDatesForProduct(product.ID)
	if err != nil {
		log.Printf("Failed to load bookings of %s for cart: %v", product.ID, err)
		return "Nie udało się sprawdzić dostępności. Spróbuj ponownie."
	}
	unavailable := mergeDateLists(product.BookedDates, dbBooked, closed)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if slices.Contains(unavailable, day.Format(dateLayout)) {
			return "Wybrane terminy nie są już dostępne."
		}
	}
	return ""
}
