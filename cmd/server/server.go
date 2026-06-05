package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/products"
)

// Server holds the application state
type Server struct {
	eventStore     eventstore.EventStore
	readModels     *projections.ReadModelsDB
	productParser  *products.Parser
	templates      *template.Template
}

// NewServer creates a new HTTP server
func NewServer(eventStore eventstore.EventStore, readModels *projections.ReadModelsDB, productParser *products.Parser) *Server {
	// Create template functions map
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
	}

	// Parse templates from filesystem
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html"))

	return &Server{
		eventStore:    eventStore,
		readModels:    readModels,
		productParser: productParser,
		templates:     tmpl,
	}
}

// RegisterRoutes sets up all HTTP routes
func (s *Server) RegisterRoutes(r chi.Router) {
	// Public routes
	r.Get("/", s.handleHome)
	r.Get("/product/{id}", s.handleProduct)
	r.Get("/product/{id}/calendar", s.handleProductCalendar)
	r.Get("/login", s.handleLogin)
	r.Get("/checkout", s.handleCheckout)
	r.Get("/payment/{id}", s.handlePayment)
	
	// Cart API (HTMX)
	r.Post("/cart/add", s.handleCartAdd)
	r.Post("/cart/remove/{id}", s.handleCartRemove)
	
	// Checkout API (HTMX)
	r.Post("/checkout/submit", s.handleCheckoutSubmit)
	
	// Payment API (HTMX)
	r.Post("/payment/confirm", s.handlePaymentConfirm)
	r.Get("/payment/status/{id}", s.handlePaymentStatus)
	
	// User panel (protected)
	r.Get("/user", s.handleUserPanel)
	r.Post("/user/delete-request", s.handleUserDeleteRequest)
	r.Post("/user/delete-confirm", s.handleUserDeleteConfirm)
	r.Post("/user/delete-cancel", s.handleUserDeleteCancel)
	
	// Admin panel (protected)
	r.Group(func(r chi.Router) {
		r.Use(s.adminAuthMiddleware)
		r.Get("/admin", s.handleAdminPanel)
		r.Get("/admin/user/{id}", s.handleAdminUserDetail)
		r.Post("/admin/order/{id}/mark-paid", s.handleAdminOrderMarkPaid)
		r.Post("/admin/transfer/{id}/link", s.handleAdminTransferLink)
	})
	
	// Health check
	r.Get("/health", s.handleHealth)
}

// handleHome renders the home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	products, err := s.productParser.LoadAllProducts()
	if err != nil {
		log.Printf("Error loading products: %v", err)
		http.Error(w, "Failed to load products", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title":     "Wynajem sprzętu rowerowego",
		"Products":  products,
		"CartCount": getCartCount(r),
		"CartTotal": getCartTotal(r),
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handleProduct renders the product detail page
func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")
	
	if productID == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	product, err := s.productParser.LoadProductByID(productID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	now := time.Now()
	data := map[string]interface{}{
		"Title":         product.Name,
		"Product":       product,
		"CurrentMonth":  int(now.Month()),
		"CurrentYear":   now.Year(),
		"CartCount":     getCartCount(r),
		"CartTotal":     getCartTotal(r),
	}

	s.renderTemplate(w, "base.html", data, nil)
}


// handleProductCalendar renders the calendar widget (HTMX)
func (s *Server) handleProductCalendar(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")

	product, err := s.productParser.LoadProductByID(productID)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	// Parse query parameters
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	if month == 0 {
		month = int(time.Now().Month())
	}
	if year == 0 {
		year = time.Now().Year()
	}

	// Generate calendar grid
	calendarGrid := s.generateCalendarGrid(year, month, product.BookedDates, startDate, endDate)

	// Calculate rental days
	rentalDays := 1
	if startDate != "" && endDate != "" {
		start, _ := time.Parse("2006-01-02", startDate)
		end, _ := time.Parse("2006-01-02", endDate)
		rentalDays = int(end.Sub(start)/(24*time.Hour)) + 1
	}

	monthNames := []string{"Styczeń", "Luty", "Marzec", "Kwiecień", "Maj", "Czerwiec", "Lipiec", "Sierpień", "Wrzesień", "Październik", "Listopad", "Grudzień"}

	data := map[string]interface{}{
		"ProductID":        productID,
		"Month":            month,
		"Year":             year,
		"MonthName":        monthNames[month-1],
		"PrevMonth":        month - 1,
		"PrevYear":         year,
		"NextMonth":        month + 1,
		"NextYear":         year,
		"CalendarGrid":    calendarGrid,
		"SelectedStartDate": startDate,
		"SelectedEndDate":   endDate,
		"RentalDays":       rentalDays,
	}

	s.renderTemplate(w, "calendar.html", data, nil)
}

// generateCalendarGrid generates the calendar grid for a given month
func (s *Server) generateCalendarGrid(year, month int, bookedDates []string, startDate, endDate string) [][]CalendarDay {
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstDay.AddDate(0, 1, -1)
	
	// Get first weekday (Monday = 0)
	firstWeekday := int(firstDay.Weekday())
	if firstWeekday == 0 {
		firstWeekday = 6
	} else {
		firstWeekday--
	}
	
	var grid [][]CalendarDay
	var row []CalendarDay
	
	// Add empty cells for days before the first of the month
	for i := 0; i < firstWeekday; i++ {
		row = append(row, CalendarDay{Empty: true})
	}
	
	// Add days of the month
	now := time.Now().Truncate(24 * time.Hour)
	for day := 1; day <= lastDay.Day(); day++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		currentDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		
		isBooked := false
		for _, bd := range bookedDates {
			if bd == dateStr {
				isBooked = true
				break
			}
		}
		
		isSelected := dateStr == startDate || dateStr == endDate
		isBetween := false
		if startDate != "" && endDate != "" {
			start, _ := time.Parse("2006-01-02", startDate)
			end, _ := time.Parse("2006-01-02", endDate)
			if currentDate.After(start) && currentDate.Before(end) {
				isBetween = true
			}
		}
		
		isPast := currentDate.Before(now)
		
		row = append(row, CalendarDay{
			Day:       day,
			DateStr:   dateStr,
			Empty:     false,
			IsBooked:  isBooked,
			IsSelected: isSelected,
			IsBetween: isBetween,
			IsPast:    isPast,
		})
		
		if len(row) == 7 {
			grid = append(grid, row)
			row = []CalendarDay{}
		}
	}
	
	// Add remaining cells
	if len(row) > 0 {
		for len(row) < 7 {
			row = append(row, CalendarDay{Empty: true})
		}
		grid = append(grid, row)
	}
	
	return grid
}

// CalendarDay represents a single day in the calendar
type CalendarDay struct {
	Day       int
	DateStr   string
	Empty     bool
	IsBooked  bool
	IsSelected bool
	IsBetween bool
	IsPast    bool
}

// handleLogin renders the login page
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// Handle login (simplified for now)
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}

	data := map[string]interface{}{
		"Title": "Zaloguj się",
	}

	s.renderTemplate(w, "login.html", data, nil)
}

// handleCheckout renders the checkout page
func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	cart := getCart(r)
	
	data := map[string]interface{}{
		"Title":        "Koszyk",
		"Cart":         cart,
		"CartTotal":    calculateCartTotal(cart),
		"AddonsTotal":  calculateAddonsTotal(cart),
		"FinalTotal":   calculateFinalTotal(cart),
		"CartCount":    len(cart),
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handlePayment renders the payment page
func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	// Simplified payment handling
	data := map[string]interface{}{
		"Title":         "Płatność",
		"PaymentMethod": "blik",
		"FinalTotal":    150,
		"OrderID":       "1234",
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handleCartAdd adds an item to the cart (HTMX)
func (s *Server) handleCartAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	productID := r.FormValue("product_id")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")
	rentalDays, _ := strconv.Atoi(r.FormValue("rental_days"))

	// Get addons
	var addons []string
	for _, addon := range r.Form["addons"] {
		addons = append(addons, addon)
	}

	// Load product
	product, err := s.productParser.LoadProductByID(productID)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	// Add to cart session
	cart := getCart(r)
	cartItem := CartItem{
		CartID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		ProductID:   productID,
		ProductName: product.Name,
		BasePrice:   product.BasePrice,
		StartDate:   startDate,
		EndDate:     endDate,
		RentalDays:  rentalDays,
		Addons:      addons,
		Total:       calculateItemTotal(product, addons, rentalDays),
	}
	cart = append(cart, cartItem)
	setCart(w, r, cart)

	// Redirect to checkout
	http.Redirect(w, r, "/checkout", http.StatusFound)
}

// handleCartRemove removes an item from the cart (HTMX)
func (s *Server) handleCartRemove(w http.ResponseWriter, r *http.Request) {
	cartID := chi.URLParam(r, "id")
	cart := getCart(r)
	
	var newCart []CartItem
	for _, item := range cart {
		if item.CartID != cartID {
			newCart = append(newCart, item)
		}
	}
	
	setCart(w, r, newCart)
	
	// Return updated cart content
	s.handleCheckout(w, r)
}

// handleCheckoutSubmit processes the checkout form (HTMX)
func (s *Server) handleCheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Process order (simplified)
	// In real implementation, this would:
	// 1. Validate form data
	// 2. Create user if needed
	// 3. Emit OrderPlaced event
	// 4. Clear cart
	// 5. Redirect to payment

	paymentMethod := r.FormValue("payment_method")
	redirectURL := fmt.Sprintf("/payment?method=%s", paymentMethod)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handlePaymentConfirm confirms payment (HTMX)
func (s *Server) handlePaymentConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear cart
	clearCart(w, r)

	// Redirect to success page
	http.Redirect(w, r, "/success", http.StatusFound)
}

// handlePaymentStatus checks payment status (HTMX polling)
func (s *Server) handlePaymentStatus(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	
	// In real implementation, check order status from read models
	// For now, simulate payment confirmation after a few checks
	_ = orderID
	
	// Return success fragment
	data := map[string]interface{}{
		"Email": "user@example.com",
	}
	s.renderTemplate(w, "success.html", data, nil)
}

// handleUserPanel renders the user panel
func (s *Server) handleUserPanel(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":      "Panel Klienta",
		"User":       map[string]interface{}{"Name": "Jan Kowalski", "Email": "jan@example.com", "Phone": "500 111 222", "Address": "Warszawska 1, Radzymin"},
		"Orders":     []interface{}{},
		"DeleteState": "idle",
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handleUserDeleteRequest initiates account deletion (HTMX)
func (s *Server) handleUserDeleteRequest(w http.ResponseWriter, r *http.Request) {
	// Return confirmation dialog
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div id="delete-section" class="bg-red-50 p-4 rounded-xl border border-red-100">
		<h4 class="font-bold text-red-900 mb-1">Czy na pewno?</h4>
		<p class="text-xs text-red-700 mb-3 leading-relaxed">
			Wysyłasz żądanie usunięcia wszystkich swoich danych osobowych oraz historii zamówień z naszego systemu. Tej operacji nie można cofnąć.
		</p>
		<div class="flex space-x-2">
			<button hx-post="/user/delete-confirm" hx-target="#delete-section" class="bg-red-600 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-700 transition-colors">Tak, usuń konto</button>
			<button hx-post="/user/delete-cancel" hx-target="#delete-section" class="bg-white border border-red-200 text-red-800 px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-50 transition-colors">Anuluj</button>
		</div>
	</div>`)
}

// handleUserDeleteConfirm confirms account deletion (HTMX)
func (s *Server) handleUserDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div id="delete-section" class="bg-emerald-50 text-emerald-800 p-4 rounded-xl text-sm flex items-center font-medium border border-emerald-100">
		<svg class="w-5 h-5 mr-2 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
		</svg>
		Wysłano prośbę o usunięcie. Administrator przetworzy ją w ciągu 14 dni.
	</div>`)
}

// handleUserDeleteCancel cancels account deletion (HTMX)
func (s *Server) handleUserDeleteCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<button hx-post="/user/delete-request" hx-target="#delete-section" class="text-red-500 text-sm hover:underline font-medium">
		Zażądaj usunięcia konta (RODO)
	</button>`)
}

// handleAdminPanel renders the admin panel
func (s *Server) handleAdminPanel(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":     "Panel Administratora",
		"Orders":    []interface{}{},
		"Transfers": []interface{}{},
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handleAdminUserDetail renders admin user detail page
func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	
	data := map[string]interface{}{
		"Title":      "Szczegóły użytkownika",
		"User":       map[string]interface{}{"ID": userID, "Name": "Jan Kowalski", "Email": "jan@example.com", "Phone": "500 111 222", "Address": "Warszawska 1, Radzymin"},
		"UserOrders": []interface{}{},
	}

	s.renderTemplate(w, "base.html", data, nil)
}

// handleAdminOrderMarkPaid marks an order as paid (HTMX)
func (s *Server) handleAdminOrderMarkPaid(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	
	// In real implementation, emit OrderPaid event
	_ = orderID
	
	// Return updated orders section
	s.handleAdminPanel(w, r)
}

// handleAdminTransferLink links a transfer to an order (HTMX)
func (s *Server) handleAdminTransferLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := chi.URLParam(r, "id")
	
	orderID := r.FormValue("value")
	
	// In real implementation, emit TransferLinked and OrderPaid events
	_ = transferID
	_ = orderID
	
	// Return updated transfers section
	s.handleAdminPanel(w, r)
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Helper functions

// adminAuthMiddleware checks if user is authenticated as admin
func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for admin session cookie
		cookie, err := r.Cookie("admin_session")
		if err != nil || cookie.Value != "authenticated" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}, contentFn func()) {
	// Add default values for authentication and cart
	if dataMap, ok := data.(map[string]interface{}); ok {
		if _, hasKey := dataMap["IsLoggedIn"]; !hasKey {
			dataMap["IsLoggedIn"] = false
		}
		if _, hasKey := dataMap["IsAdmin"]; !hasKey {
			dataMap["IsAdmin"] = false
		}
		if _, hasKey := dataMap["CartCount"]; !hasKey {
			dataMap["CartCount"] = 0
		}
		if _, hasKey := dataMap["CartTotal"]; !hasKey {
			dataMap["CartTotal"] = 0
		}
	}

	if contentFn != nil {
		// For nested templates, execute the content function first
		// This is a simplified approach - in production, use proper template composition
		contentFn()
		return
	}
	
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Cart types and functions

type CartItem struct {
	CartID      string
	ProductID   string
	ProductName string
	BasePrice   int
	StartDate   string
	EndDate     string
	RentalDays  int
	Addons      []string
	Total       int
}

func getCart(r *http.Request) []CartItem {
	if cookie, err := r.Cookie("cart"); err == nil {
		var cart []CartItem
		json.Unmarshal([]byte(cookie.Value), &cart)
		return cart
	}
	return []CartItem{}
}

func setCart(w http.ResponseWriter, r *http.Request, cart []CartItem) {
	data, _ := json.Marshal(cart)
	http.SetCookie(w, &http.Cookie{
		Name:     "cart",
		Value:    string(data),
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCart(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cart",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getCartCount(r *http.Request) int {
	return len(getCart(r))
}

func getCartTotal(r *http.Request) int {
	return calculateFinalTotal(getCart(r))
}

func calculateItemTotal(product *domain.Product, addons []string, rentalDays int) int {
	total := product.BasePrice
	for _, addonID := range addons {
		for _, addon := range product.Addons {
			if addon.ID == addonID {
				total += addon.Price
			}
		}
	}
	return total * rentalDays
}

func calculateCartTotal(cart []CartItem) int {
	total := 0
	for _, item := range cart {
		total += item.BasePrice * item.RentalDays
	}
	return total
}

func calculateAddonsTotal(cart []CartItem) int {
	total := 0
	for _, item := range cart {
		// Simplified - in real implementation, calculate addon prices
		_ = item
	}
	return total
}

func calculateFinalTotal(cart []CartItem) int {
	return calculateCartTotal(cart) + calculateAddonsTotal(cart)
}
