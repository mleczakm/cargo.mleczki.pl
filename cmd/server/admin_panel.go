package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/middleware"
	"cargo.mleczki.pl/internal/projections"
)

const (
	maxClosureSpanDays = 366
	adminTabOrders     = "orders"
	adminTabFleet      = "fleet"
	adminTabClosures   = "closures"
)

var polishWeekdays = [...]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"}

// adminBack redirects to an admin tab with a flash message. Admin actions are
// plain form posts that come back here, so the page is never swapped piecemeal.
func (s *Server) adminBack(w http.ResponseWriter, r *http.Request, tab, kind, message string) {
	// Events are projected in the background; catch up now so the page shows what the admin just did.
	if err := s.projector.Run(r.Context()); err != nil {
		log.Printf("Projection after admin action failed: %v", err)
	}
	http.Redirect(w, r, "/admin?"+kind+"="+url.QueryEscape(message)+"#"+tab, http.StatusSeeOther)
}

// parseAdminDateRange reads "date" and the optional "end_date" (inclusive).
func parseAdminDateRange(r *http.Request) ([]string, error) {
	start, err := time.Parse(dateLayout, strings.TrimSpace(r.FormValue("date")))
	if err != nil {
		return nil, errors.New("Podaj poprawną datę.")
	}
	end := start
	if raw := strings.TrimSpace(r.FormValue("end_date")); raw != "" {
		if end, err = time.Parse(dateLayout, raw); err != nil {
			return nil, errors.New("Podaj poprawną datę końcową.")
		}
	}
	if end.Before(start) {
		return nil, errors.New("Data końcowa nie może być wcześniejsza niż początkowa.")
	}
	if rentalDaysBetween(start, end) > maxClosureSpanDays {
		return nil, fmt.Errorf("Zakres może obejmować najwyżej %d dni.", maxClosureSpanDays)
	}
	var dates []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format(dateLayout))
	}
	return dates, nil
}

func todayString() string { return time.Now().Format(dateLayout) }

func plural(n int, one, few, many string) string {
	switch {
	case n == 1:
		return one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return few
	default:
		return many
	}
}

func daysLabel(n int) string { return fmt.Sprintf("%d %s", n, plural(n, "dzień", "dni", "dni")) }

// handleAdminAddGlobalClosure closes the whole store for a day or a range of days.
func (s *Server) handleAdminAddGlobalClosure(w http.ResponseWriter, r *http.Request) {
	dates, err := parseAdminDateRange(r)
	if err != nil {
		s.adminBack(w, r, adminTabClosures, "error", err.Error())
		return
	}
	if dates[0] < todayString() {
		s.adminBack(w, r, adminTabClosures, "error", "Nie można zamknąć sklepu w przeszłości.")
		return
	}
	actor := ""
	if user, ok := middleware.GetUser(r).(*domain.User); ok && user != nil {
		actor = user.ID
	}
	for _, date := range dates {
		if err := s.readModels.AddGlobalBlockedDate(date, actor); err != nil {
			log.Printf("Failed to add global closure date %s: %v", date, err)
			s.adminBack(w, r, adminTabClosures, "error", "Nie udało się zapisać zamknięcia.")
			return
		}
	}
	message := "Sklep zamknięty: " + daysLabel(len(dates)) + "."
	if existing, err := s.readModels.CountCustomerBookings(dates[0], dates[len(dates)-1]); err == nil && existing > 0 {
		message += fmt.Sprintf(" Uwaga: w tym okresie są już rezerwacje klientów (%d), pozostają w mocy.", existing)
	}
	s.adminBack(w, r, adminTabClosures, "notice", message)
}

// handleAdminRemoveGlobalClosure reopens the store for a day or a range of days.
func (s *Server) handleAdminRemoveGlobalClosure(w http.ResponseWriter, r *http.Request) {
	dates, err := parseAdminDateRange(r)
	if err != nil {
		s.adminBack(w, r, adminTabClosures, "error", err.Error())
		return
	}
	for _, date := range dates {
		if err := s.readModels.RemoveGlobalBlockedDate(date); err != nil {
			log.Printf("Failed to remove global closure date %s: %v", date, err)
			s.adminBack(w, r, adminTabClosures, "error", "Nie udało się usunąć zamknięcia.")
			return
		}
	}
	s.adminBack(w, r, adminTabClosures, "notice", "Sklep ponownie otwarty: "+daysLabel(len(dates))+".")
}

func (s *Server) handleAdminBlockProductDate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminProductDateAction(w, r, true)
}

func (s *Server) handleAdminUnblockProductDate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminProductDateAction(w, r, false)
}

// handleAdminProductDateAction blocks or unblocks days of one product by hand.
func (s *Server) handleAdminProductDateAction(w http.ResponseWriter, r *http.Request, block bool) {
	productID := chi.URLParam(r, "id")
	product, err := s.productParser.LoadProductByID(productID)
	if err != nil {
		s.adminBack(w, r, adminTabFleet, "error", "Nie znaleziono sprzętu.")
		return
	}
	dates, err := parseAdminDateRange(r)
	if err != nil {
		s.adminBack(w, r, adminTabFleet, "error", err.Error())
		return
	}
	if block && dates[0] < todayString() {
		s.adminBack(w, r, adminTabFleet, "error", "Nie można blokować dni z przeszłości.")
		return
	}
	orderID := projections.AdminBlockPrefix + productID
	changed := 0
	for _, date := range dates {
		var res sql.Result
		if block {
			res, err = s.readModels.GetDB().Exec(`INSERT OR IGNORE INTO product_bookings (product_id, order_id, booked_date) VALUES (?, ?, ?)`, productID, orderID, date)
		} else {
			res, err = s.readModels.GetDB().Exec(`DELETE FROM product_bookings WHERE product_id = ? AND order_id = ? AND booked_date = ?`, productID, orderID, date)
		}
		if err != nil {
			log.Printf("Failed to update product date %s/%s: %v", productID, date, err)
			s.adminBack(w, r, adminTabFleet, "error", "Nie udało się zapisać zmiany.")
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
	}
	switch {
	case block && changed == 0:
		s.adminBack(w, r, adminTabFleet, "error", product.Name+": te dni są już zajęte.")
	case block:
		message := fmt.Sprintf("%s: zablokowano %s.", product.Name, daysLabel(changed))
		if changed < len(dates) {
			message += fmt.Sprintf(" Pominięto %d, bo były już zajęte.", len(dates)-changed)
		}
		s.adminBack(w, r, adminTabFleet, "notice", message)
	default:
		s.adminBack(w, r, adminTabFleet, "notice", fmt.Sprintf("%s: odblokowano %s.", product.Name, daysLabel(changed)))
	}
}

type closureView struct {
	Date    string
	Weekday string
	Past    bool
}

type fleetProductView struct {
	ID, Name, Image string
	Blocked         []closureView
}

func weekdayOf(date string) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return ""
	}
	return polishWeekdays[t.Weekday()]
}

func (s *Server) closureViews() ([]closureView, []closureView) {
	dates, err := s.readModels.GetGlobalBlockedDates()
	if err != nil {
		log.Printf("Error loading global blocked dates: %v", err)
		return nil, nil
	}
	var upcoming, past []closureView
	now := todayString()
	for _, date := range dates {
		view := closureView{Date: date, Weekday: weekdayOf(date), Past: date < now}
		if view.Past {
			past = append(past, view)
		} else {
			upcoming = append(upcoming, view)
		}
	}
	return upcoming, past
}

func (s *Server) fleetViews(products []domain.Product) []fleetProductView {
	blocked, err := s.readModels.GetAdminBlockedDates()
	if err != nil {
		log.Printf("Error loading admin blocked dates: %v", err)
	}
	now := todayString()
	views := make([]fleetProductView, 0, len(products))
	for _, product := range products {
		view := fleetProductView{ID: product.ID, Name: product.Name, Image: product.Image}
		for _, date := range blocked[product.ID] {
			if date >= now {
				view.Blocked = append(view.Blocked, closureView{Date: date, Weekday: weekdayOf(date)})
			}
		}
		views = append(views, view)
	}
	return views
}

// enrichAdminOrders adds Polish labels so the template shows no internal codes.
func enrichAdminOrders(orders []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, len(orders))
	for i, order := range orders {
		enriched := make(map[string]interface{}, len(order)+4)
		for k, v := range order {
			enriched[k] = v
		}
		status, _ := order["Status"].(string)
		method, _ := order["PaymentMethod"].(string)
		enriched["StatusLabel"] = orderStatusLabel(status, method)
		enriched["StatusBadgeClass"] = orderStatusBadgeClass(status, method)
		enriched["PaymentLabel"] = paymentMethodLabel(method, status)
		itemsJSON, _ := order["Items"].(string)
		enriched["ItemsSummary"] = summarizeOrderItems(itemsJSON)
		out[i] = enriched
	}
	return out
}

// handleAdminUserDetail shows one customer with the order history.
func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	user, err := s.readModels.GetUserByID(userID)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("Failed to load user %s: %v", userID, err)
		http.Error(w, "Failed to load user", http.StatusInternalServerError)
		return
	}
	orders, err := s.readModels.GetOrdersByUserID(userID)
	if err != nil {
		log.Printf("Failed to load orders of %s: %v", userID, err)
	}
	s.renderTemplate(w, r, "admin_user_detail.html", map[string]interface{}{
		"Title":      "Klient: " + fmt.Sprint(user["Name"]),
		"User":       user,
		"UserOrders": enrichAdminOrders(orders),
		"IsAdmin":    true,
		"Notice":     r.URL.Query().Get("notice"),
		"Error":      r.URL.Query().Get("error"),
	})
}

func countOrders(orders []map[string]interface{}, status string) int {
	count := 0
	for _, order := range orders {
		if order["Status"] == status {
			count++
		}
	}
	return count
}

// summarizeOrderItems turns the stored items JSON into one readable line.
func summarizeOrderItems(itemsJSON string) string {
	var parts []string
	for _, item := range parseOrderItems(itemsJSON) {
		part := item.ProductName
		switch {
		case item.IsShop() && item.Quantity > 1:
			part += fmt.Sprintf(" × %d", item.Quantity)
		case item.StartDate != "" && item.EndDate != "" && item.StartDate != item.EndDate:
			part += fmt.Sprintf(" (%s – %s)", item.StartDate, item.EndDate)
		case item.StartDate != "":
			part += fmt.Sprintf(" (%s)", item.StartDate)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}
