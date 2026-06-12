package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/email"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/transfers"
)

const methodPost = "POST"

// Server holds the application state.
type Server struct {
	eventStore      eventstore.EventStore
	readModels      *projections.ReadModelsDB
	productParser   *products.Parser
	authManager     *auth.AuthManager
	templates       *template.Template
	transferMatcher *transfers.Matcher
	emailImporter   *email.Importer
}

// partialTemplates are HTMX fragments rendered without the site layout.
var partialTemplates = map[string]struct{}{
	"calendar.html":        {},
	"payment_success.html": {},
}

// NewServer creates a new HTTP server.
func NewServer(eventStore eventstore.EventStore, readModels *projections.ReadModelsDB, productParser *products.Parser, authManager *auth.AuthManager) *Server {
	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) }, // #nosec G203 // Content is from trusted markdown files
		"stripHTML": func(s string) template.HTML {
			// Strips only outer <p> tags commonly added by markdown parser for short descriptions
			stripped := strings.ReplaceAll(strings.ReplaceAll(s, "<p>", ""), "</p>", "")
			return template.HTML(stripped) // #nosec G203 // Content is from trusted markdown files
		},
	}

	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob("web/templates/*.html"))

	// Initialize email importer if credentials are provided
	var emailImporter *email.Importer
	imapServer := os.Getenv("IMAP_SERVER")
	mailboxUsername := os.Getenv("MAILBOX_USERNAME")
	mailboxPassword := os.Getenv("MAILBOX_PASSWORD")
	if imapServer != "" && mailboxUsername != "" && mailboxPassword != "" {
		imapClient := email.NewIMAPClient(imapServer, mailboxUsername, mailboxPassword, "INBOX")
		emailParser := email.NewParser()
		emailImporter = email.NewImporter(imapClient, emailParser, eventStore, readModels.GetDB())
		log.Println("Email importer initialized")
	}

	server := &Server{
		eventStore:      eventStore,
		readModels:      readModels,
		productParser:   productParser,
		authManager:     authManager,
		templates:       tmpl,
		transferMatcher: transfers.NewMatcher(),
		emailImporter:   emailImporter,
	}

	// Start background email import scheduler if configured
	if emailImporter != nil {
		go server.startEmailImportScheduler()
	}

	return server
}

// startEmailImportScheduler runs a background scheduler that imports transfers every 30 seconds
// only if there are pending BLIK payments waiting to be matched.
func (s *Server) startEmailImportScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Email import scheduler started (runs every 30s when pending payments exist)")

	for range ticker.C {
		if s.emailImporter == nil {
			continue
		}

		// Check if there are pending payments
		hasPending, err := s.emailImporter.HasPendingPayments()
		if err != nil {
			log.Printf("Failed to check for pending payments: %v", err)
			continue
		}

		if !hasPending {
			continue
		}

		// Import transfers
		count, err := s.emailImporter.ImportTransfers(context.Background())
		if err != nil {
			log.Printf("Failed to import transfers: %v", err)
			continue
		}

		if count > 0 {
			log.Printf("Scheduled import: imported %d transfers", count)
		}
	}
}

// RegisterRoutes sets up all HTTP routes.
func (s *Server) RegisterRoutes(r chi.Router) {
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	r.Handle("/data/images/*", http.StripPrefix("/data/images/", http.FileServer(http.Dir("data/images"))))
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon-32.svg", http.StatusMovedPermanently)
	})

	// Public routes
	r.Get("/", s.handleHome)
	r.Get("/product/{id}", s.handleProduct)
	r.Get("/product/{id}/calendar", s.handleProductCalendar)
	r.Get("/login", s.handleLogin)
	r.Post("/login", s.handleLogin)
	r.Get("/logout", s.handleLogout)
	r.Post("/logout", s.handleLogout)
	r.Get("/checkout", s.handleCheckout)
	r.Get("/payment/{id}", s.handlePayment)
	r.Get("/success", s.handleSuccess)
	r.Get("/terms", s.handleTerms)

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
		r.Post("/admin/order/{id}/confirm", s.handleAdminOrderConfirm)
		r.Post("/admin/transfer/{id}/link", s.handleAdminTransferLink)
		r.Post("/admin/transfers/import", s.handleAdminImportTransfers)
		r.Post("/admin/reservation/create", s.handleAdminCreateReservation)
		r.Post("/admin/product/{id}/block-date", s.handleAdminBlockProductDate)
		r.Post("/admin/product/{id}/unblock-date", s.handleAdminUnblockProductDate)
	})

	// Health check
	r.Get("/health", s.handleHealth)
}

// handleHome renders the home page.
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

	s.renderTemplate(w, r, "home.html", data)
}

// handleProduct renders the product detail page.
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
		"Title":        product.Name,
		"Product":      product,
		"CurrentMonth": int(now.Month()),
		"CurrentYear":  now.Year(),
		"CartCount":    getCartCount(r),
		"CartTotal":    getCartTotal(r),
	}

	s.renderTemplate(w, r, "product.html", data)
}

// handleProductCalendar renders the calendar widget (HTMX).
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

	// Fetch global blocked dates
	globalBlockedDates, err := s.readModels.GetGlobalBlockedDates()
	if err != nil {
		log.Printf("Error loading global blocked dates: %v", err)
		globalBlockedDates = []string{}
	}

	// Generate calendar grid
	calendarGrid := s.generateCalendarGrid(year, month, product.BookedDates, globalBlockedDates, startDate, endDate)

	// Calculate rental days
	rentalDays := 1
	if startDate != "" && endDate != "" {
		start, _ := time.Parse("2006-01-02", startDate)
		end, _ := time.Parse("2006-01-02", endDate)
		rentalDays = int(end.Sub(start)/(24*time.Hour)) + 1
	}

	monthNames := []string{"Styczeń", "Luty", "Marzec", "Kwiecień", "Maj", "Czerwiec", "Lipiec", "Sierpień", "Wrzesień", "Październik", "Listopad", "Grudzień"}

	data := map[string]interface{}{
		"ProductID":         productID,
		"Month":             month,
		"Year":              year,
		"MonthName":         monthNames[month-1],
		"PrevMonth":         month - 1,
		"PrevYear":          year,
		"NextMonth":         month + 1,
		"NextYear":          year,
		"CalendarGrid":      calendarGrid,
		"SelectedStartDate": startDate,
		"SelectedEndDate":   endDate,
		"RentalDays":        rentalDays,
	}

	s.renderTemplate(w, r, "calendar.html", data)
}

// generateCalendarGrid generates the calendar grid for a given month.
func (s *Server) generateCalendarGrid(year, month int, bookedDates []string, globalBlockedDates []string, startDate, endDate string) [][]CalendarDay {
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstDay.AddDate(0, 1, -1)

	firstWeekday := s.getMondayBasedWeekday(firstDay.Weekday())
	now := time.Now().Truncate(24 * time.Hour)

	grid := make([][]CalendarDay, 0)
	row := make([]CalendarDay, 0, firstWeekday)

	// Add empty cells for days before the first of the month
	for i := 0; i < firstWeekday; i++ {
		row = append(row, CalendarDay{Empty: true})
	}

	// Add days of the month
	for day := 1; day <= lastDay.Day(); day++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
		currentDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

		isBooked := s.isDateBooked(dateStr, bookedDates, globalBlockedDates)
		isSelected := dateStr == startDate || dateStr == endDate
		isBetween := s.isDateBetween(dateStr, startDate, endDate)
		isPast := currentDate.Before(now)

		row = append(row, CalendarDay{
			Day:        day,
			DateStr:    dateStr,
			Empty:      false,
			IsBooked:   isBooked,
			IsSelected: isSelected,
			IsBetween:  isBetween,
			IsPast:     isPast,
		})

		if len(row) == 7 {
			grid = append(grid, row)
			row = make([]CalendarDay, 0)
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

// getMondayBasedWeekday converts Sunday=0 to Monday=0 format.
func (s *Server) getMondayBasedWeekday(weekday time.Weekday) int {
	if weekday == time.Sunday {
		return 6
	}
	return int(weekday) - 1
}

// isDateBooked checks if a date is booked (product-specific or global).
func (s *Server) isDateBooked(dateStr string, bookedDates, globalBlockedDates []string) bool {
	for _, bd := range bookedDates {
		if bd == dateStr {
			return true
		}
	}
	for _, gbd := range globalBlockedDates {
		if gbd == dateStr {
			return true
		}
	}
	return false
}

// isDateBetween checks if a date is between start and end dates.
func (s *Server) isDateBetween(dateStr, startDate, endDate string) bool {
	if startDate == "" || endDate == "" {
		return false
	}
	currentDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return false
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return false
	}
	return currentDate.After(start) && currentDate.Before(end)
}

// CalendarDay represents a single day in the calendar.
type CalendarDay struct {
	Day        int
	DateStr    string
	Empty      bool
	IsBooked   bool
	IsSelected bool
	IsBetween  bool
	IsPast     bool
}

// handleLogin renders the login page and handles login submission.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	errorMessage := ""
	if r.Method == methodPost {
		email := r.FormValue("email")
		password := r.FormValue("password")

		if email == "" || password == "" {
			errorMessage = "Email and password are required"
		} else {
			ctx := r.Context()
			sessionToken, user, err := s.authManager.Login(ctx, email, password)
			if err != nil {
				errorMessage = "Invalid email or password"
				log.Printf("Login failed for email %s: %v", email, err)
			} else {
				// Set session cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "session_token",
					Value:    sessionToken,
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteStrictMode,
					MaxAge:   30 * 24 * 60 * 60, // 30 days
				})

				// Redirect based on user role
				if user.IsAdmin {
					w.Header().Set("HX-Redirect", "/admin")
				} else {
					w.Header().Set("HX-Redirect", "/user")
				}
				return
			}
		}
	}

	data := map[string]interface{}{
		"Title":        "Zaloguj się",
		"ErrorMessage": errorMessage,
	}

	// If modal request, return just the content
	if r.URL.Query().Get("modal") == "1" {
		if err := s.templates.ExecuteTemplate(w, "login-content", data); err != nil {
			log.Printf("Template error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// For POST requests with errors, also return just the content for HTMX
	if r.Method == methodPost && errorMessage != "" {
		if err := s.templates.ExecuteTemplate(w, "login-content", data); err != nil {
			log.Printf("Template error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	s.renderTemplate(w, r, "login.html", data)
}

// handleLogout handles user logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		ctx := r.Context()
		_ = s.authManager.Logout(ctx, cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	// Redirect to home
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleCheckout renders the checkout page.
func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	cart := getCart(r)

	data := map[string]interface{}{
		"Title":       "Koszyk",
		"Cart":        cart,
		"CartTotal":   calculateCartTotal(cart),
		"AddonsTotal": calculateAddonsTotal(cart),
		"FinalTotal":  calculateFinalTotal(cart),
		"CartCount":   len(cart),
		"UserDetails": map[string]string{},
	}

	s.renderTemplate(w, r, "checkout.html", data)
}

// handlePayment renders the payment page.
func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	orderID := r.URL.Query().Get("id")
	paymentMethod := r.URL.Query().Get("method")

	if orderID == "" {
		http.Error(w, "Order ID required", http.StatusBadRequest)
		return
	}

	// Get order details
	var totalAmount float64
	var status, dbPaymentMethod string
	var paymentCode *string
	err := s.readModels.GetDB().QueryRow(`
		SELECT total_amount, status, payment_method, payment_code
		FROM orders WHERE id = ?
	`, orderID).Scan(&totalAmount, &status, &dbPaymentMethod, &paymentCode)
	if err != nil {
		log.Printf("Failed to get order: %v", err)
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Use payment method from query param or database
	if paymentMethod == "" {
		paymentMethod = dbPaymentMethod
	}

	data := map[string]interface{}{
		"Title":         "Płatność",
		"PaymentMethod": paymentMethod,
		"FinalTotal":    int(totalAmount),
		"OrderID":       orderID,
		"PaymentCode":   paymentCode,
	}

	s.renderTemplate(w, r, "payment.html", data)
}

// handleCartAdd adds an item to the cart (HTMX).
func (s *Server) handleCartAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
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

// handleCartRemove removes an item from the cart (HTMX).
func (s *Server) handleCartRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cartID := chi.URLParam(r, "id")
	cart := getCart(r)

	var newCart []CartItem
	found := false
	for _, item := range cart {
		if item.CartID != cartID {
			newCart = append(newCart, item)
		} else {
			found = true
		}
	}

	if !found {
		http.Error(w, "Item not found in cart", http.StatusNotFound)
		return
	}

	setCart(w, r, newCart)

	// Redirect back to checkout to show updated cart
	http.Redirect(w, r, "/checkout", http.StatusFound)
}

// handleCheckoutSubmit processes the checkout form (HTMX).
//
//nolint:gocyclo // Function complexity is acceptable for this handler
func (s *Server) handleCheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get form data
	paymentMethod := r.FormValue("payment_method")
	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	isAdult := r.FormValue("is_adult") == "on"
	acceptTOS := r.FormValue("accept_tos") == "on"

	// Validate required fields
	if name == "" || email == "" || phone == "" || address == "" || !isAdult || !acceptTOS {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Get cart
	cart := getCart(r)
	if len(cart) == 0 {
		http.Error(w, "Cart is empty", http.StatusBadRequest)
		return
	}

	// Calculate total
	totalAmount := calculateFinalTotal(cart)

	// Generate order ID
	orderID := fmt.Sprintf("ORD-%d", time.Now().UnixNano())

	// Generate payment code for BLIK payments
	var paymentCode *string
	if paymentMethod == "blik" {
		code := domain.GeneratePaymentCode()
		paymentCode = &code
	}

	// Create user (simplified - in real implementation would check if user exists)
	userID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	ctx := r.Context()

	// Hash password if provided
	var passwordHash string
	if password := r.FormValue("password"); password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to process password", http.StatusInternalServerError)
			return
		}
		passwordHash = string(hash)
	}

	// Insert user
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.readModels.GetDB().ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, email, passwordHash, name, phone, address, 1, 1, 0, now, now)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		// User might already exist, try to get existing user
		err = s.readModels.GetDB().QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", email).Scan(&userID)
		if err != nil {
			http.Error(w, "Failed to create/get user", http.StatusInternalServerError)
			return
		}
	}

	// Insert order
	_, err = s.readModels.GetDB().ExecContext(ctx, `
		INSERT INTO orders (id, user_id, total_amount, status, payment_method, payment_code, items_json, created_at, updated_at)
		VALUES (?, ?, ?, 'pending_payment', ?, ?, ?, ?, ?)
	`, orderID, userID, totalAmount, paymentMethod, paymentCode, "", now, now)
	if err != nil {
		log.Printf("Failed to create order: %v", err)
		http.Error(w, "Failed to create order", http.StatusInternalServerError)
		return
	}

	// Create payment code record if BLIK
	if paymentMethod == "blik" && paymentCode != nil {
		paymentCodeID := fmt.Sprintf("pc_%d", time.Now().UnixNano())
		err = s.readModels.CreatePaymentCode(paymentCodeID, *paymentCode, orderID)
		if err != nil {
			log.Printf("Failed to create payment code: %v", err)
		}
	}

	// Block dates in product_bookings table
	for _, item := range cart {
		start, _ := time.Parse("2006-01-02", item.StartDate)
		end, _ := time.Parse("2006-01-02", item.EndDate)
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			bookingQuery := `
			INSERT INTO product_bookings (product_id, order_id, booked_date)
			VALUES (?, ?, ?)
			`
			_, err = s.readModels.GetDB().Exec(bookingQuery, item.ProductID, orderID, dateStr)
			if err != nil {
				log.Printf("Failed to block date %s: %v", dateStr, err)
			}
		}
	}

	// Clear cart
	clearCart(w, r)

	// Redirect to payment
	redirectURL := fmt.Sprintf("/payment?id=%s&method=%s", orderID, paymentMethod)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handlePaymentConfirm confirms payment (HTMX).
func (s *Server) handlePaymentConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := r.URL.Query().Get("id")
	if orderID == "" {
		http.Error(w, "Order ID required", http.StatusBadRequest)
		return
	}

	// Mark order as confirmed (for cash payments)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.readModels.GetDB().Exec(`
		UPDATE orders SET status = 'confirmed', updated_at = ?
		WHERE id = ?
	`, now, orderID)
	if err != nil {
		log.Printf("Failed to confirm order: %v", err)
		http.Error(w, "Failed to confirm order", http.StatusInternalServerError)
		return
	}

	// Clear cart
	clearCart(w, r)

	// Redirect to success page
	http.Redirect(w, r, "/success", http.StatusFound)
}

// handlePaymentStatus checks payment status (HTMX polling).
func (s *Server) handlePaymentStatus(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	// Check order status from read models
	var status string
	err := s.readModels.GetDB().QueryRow(`
		SELECT status FROM orders WHERE id = ?
	`, orderID).Scan(&status)
	if err != nil {
		log.Printf("Failed to get order status: %v", err)
		http.Error(w, "Failed to check status", http.StatusInternalServerError)
		return
	}

	// If order is paid, return success fragment
	if status == "paid" {
		data := map[string]interface{}{
			"Email": "user@example.com",
		}
		s.renderTemplate(w, r, "payment_success.html", data)
		return
	}

	// Otherwise, return loading state
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="flex flex-col items-center justify-center py-4">
		<svg class="w-10 h-10 text-emerald-600 animate-spin mb-4" fill="none" viewBox="0 0 24 24">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
		</svg>
		<p class="font-bold text-gray-900">Nasłuchujemy na przelew z banku...</p>
	</div>`)
}

// handleSuccess renders the order success page.
func (s *Server) handleSuccess(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Potwierdzenie zamówienia",
		"Email": "user@example.com",
	}

	s.renderTemplate(w, r, "success.html", data)
}

// handleTerms renders the terms modal content (HTMX).
func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if err := s.templates.ExecuteTemplate(w, "terms.html", nil); err != nil {
		log.Printf("Template error (terms.html): %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleUserPanel renders the user panel.
func (s *Server) handleUserPanel(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":       "Panel Klienta",
		"User":        map[string]interface{}{"Name": "Jan Kowalski", "Email": "jan@example.com", "Phone": "500 111 222", "Address": "Warszawska 1, Radzymin"},
		"Orders":      []interface{}{},
		"DeleteState": "idle",
	}

	data["IsLoggedIn"] = true
	s.renderTemplate(w, r, "user_panel.html", data)
}

// handleUserDeleteRequest initiates account deletion (HTMX).
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

// handleUserDeleteConfirm confirms account deletion (HTMX).
func (s *Server) handleUserDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div id="delete-section" class="bg-emerald-50 text-emerald-800 p-4 rounded-xl text-sm flex items-center font-medium border border-emerald-100">
		<svg class="w-5 h-5 mr-2 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
		</svg>
		Wysłano prośbę o usunięcie. Administrator przetworzy ją w ciągu 14 dni.
	</div>`)
}

// handleUserDeleteCancel cancels account deletion (HTMX).
func (s *Server) handleUserDeleteCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<button hx-post="/user/delete-request" hx-target="#delete-section" class="text-red-500 text-sm hover:underline font-medium">
		Zażądaj usunięcia konta (RODO)
	</button>`)
}

// handleAdminPanel renders the admin panel.
func (s *Server) handleAdminPanel(w http.ResponseWriter, r *http.Request) {
	// Fetch products
	products, err := s.productParser.LoadAllProducts()
	if err != nil {
		log.Printf("Error loading products: %v", err)
		products = []domain.Product{}
	}

	// Fetch users from read models
	users, err := s.readModels.GetAllUsers()
	if err != nil {
		log.Printf("Error loading users: %v", err)
		users = []map[string]interface{}{}
	}

	// Fetch orders from read models
	orders, err := s.readModels.GetAllOrders()
	if err != nil {
		log.Printf("Error loading orders: %v", err)
		orders = []map[string]interface{}{}
	}

	// Fetch transfers from read models (mock for now)
	transfers := []map[string]interface{}{}

	// Fetch last email import metadata
	lastEmailImport, err := s.readModels.GetLastEmailImport()
	if err != nil {
		log.Printf("Error loading last email import: %v", err)
		lastEmailImport = nil
	}

	// Fetch global blocked dates
	globalBlockedDates, err := s.readModels.GetGlobalBlockedDates()
	if err != nil {
		log.Printf("Error loading global blocked dates: %v", err)
		globalBlockedDates = []string{}
	}

	data := map[string]interface{}{
		"Title":              "Panel Administratora",
		"Orders":             orders,
		"Transfers":          transfers,
		"Users":              users,
		"Products":           products,
		"GlobalBlockedDates": globalBlockedDates,
		"LastEmailImport":    lastEmailImport,
	}

	data["IsAdmin"] = true
	s.renderTemplate(w, r, "admin_panel.html", data)
}

// handleAdminUserDetail renders admin user detail page.
func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	data := map[string]interface{}{
		"Title":      "Szczegóły użytkownika",
		"User":       map[string]interface{}{"ID": userID, "Name": "Jan Kowalski", "Email": "jan@example.com", "Phone": "500 111 222", "Address": "Warszawska 1, Radzymin"},
		"UserOrders": []interface{}{},
	}

	data["IsAdmin"] = true
	s.renderTemplate(w, r, "admin_user_detail.html", data)
}

// handleAdminOrderMarkPaid marks an order as paid (HTMX).
func (s *Server) handleAdminOrderMarkPaid(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	// Update order status to paid in read models
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
	UPDATE orders
	SET status = 'paid', paid_at = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.readModels.GetDB().Exec(query, now, now, orderID)
	if err != nil {
		log.Printf("Failed to mark order as paid: %v", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	// Return updated orders section
	s.handleAdminPanel(w, r)
}

// handleAdminOrderConfirm confirms an order manually (HTMX).
func (s *Server) handleAdminOrderConfirm(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	// Update order status to confirmed in read models
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
	UPDATE orders
	SET status = 'confirmed', updated_at = ?
	WHERE id = ?
	`
	_, err := s.readModels.GetDB().Exec(query, now, orderID)
	if err != nil {
		log.Printf("Failed to confirm order: %v", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	// Return updated orders section
	s.handleAdminPanel(w, r)
}

// handleAdminTransferLink links a transfer to an order (HTMX).
func (s *Server) handleAdminTransferLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := chi.URLParam(r, "id")
	orderID := r.FormValue("value")

	// Get transfer details
	var title string
	err := s.readModels.GetDB().QueryRow(`
		SELECT title FROM transfers WHERE id = ?
	`, transferID).Scan(&title)
	if err != nil {
		log.Printf("Failed to get transfer: %v", err)
		http.Error(w, "Transfer not found", http.StatusNotFound)
		return
	}

	// If order ID is provided manually, use it
	if orderID != "" {
		// Link transfer to order
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.readModels.GetDB().Exec(`
			UPDATE transfers SET order_id = ?, status = 'matched', linked_at = ?
			WHERE id = ?
		`, orderID, now, transferID)
		if err != nil {
			log.Printf("Failed to link transfer: %v", err)
			http.Error(w, "Failed to link transfer", http.StatusInternalServerError)
			return
		}

		// Mark order as paid
		_, err = s.readModels.GetDB().Exec(`
			UPDATE orders SET status = 'paid', paid_at = ?, updated_at = ?
			WHERE id = ?
		`, now, now, orderID)
		if err != nil {
			log.Printf("Failed to mark order as paid: %v", err)
		}
	} else {
		// Automatic matching by payment code
		// Get all pending payment codes
		rows, err := s.readModels.GetDB().Query(`
			SELECT code FROM payment_codes
		`)
		if err != nil {
			log.Printf("Failed to get payment codes: %v", err)
		} else {
			defer rows.Close()
			var codes []string
			for rows.Next() {
				var code string
				if err := rows.Scan(&code); err != nil {
					continue
				}
				codes = append(codes, code)
			}
			if err := rows.Err(); err != nil {
				log.Printf("Error iterating payment codes: %v", err)
			}

			// Try to match by payment code
			matchedCode := s.transferMatcher.MatchByPaymentCode(title, codes)
			if matchedCode != "" {
				// Get order ID by payment code
				var matchedOrderID string
				err = s.readModels.GetDB().QueryRow(`
					SELECT order_id FROM payment_codes WHERE code = ?
				`, matchedCode).Scan(&matchedOrderID)
				if err == nil {
					// Link transfer to order
					now := time.Now().UTC().Format(time.RFC3339)
					_, err = s.readModels.GetDB().Exec(`
						UPDATE transfers SET order_id = ?, status = 'matched', linked_at = ?
						WHERE id = ?
					`, matchedOrderID, now, transferID)
					if err != nil {
						log.Printf("Failed to link transfer: %v", err)
					}

					// Mark order as paid
					_, err = s.readModels.GetDB().Exec(`
						UPDATE orders SET status = 'paid', paid_at = ?, updated_at = ?
						WHERE id = ?
					`, now, now, matchedOrderID)
					if err != nil {
						log.Printf("Failed to mark order as paid: %v", err)
					}
				}
			}
		}
	}

	// Return updated transfers section
	s.handleAdminPanel(w, r)
}

// handleAdminImportTransfers triggers manual transfer import from email (HTMX).
func (s *Server) handleAdminImportTransfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.emailImporter == nil {
		http.Error(w, "Email importer not configured. Set IMAP_SERVER, IMAP_USERNAME, and IMAP_PASSWORD environment variables.", http.StatusInternalServerError)
		return
	}

	count, err := s.emailImporter.ImportTransfers(r.Context())
	if err != nil {
		log.Printf("Failed to import transfers: %v", err)
		http.Error(w, fmt.Sprintf("Failed to import transfers: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully imported %d transfers from email", count)

	// Return updated admin panel
	s.handleAdminPanel(w, r)
}

// handleAdminCreateReservation creates a manual reservation (HTMX).
func (s *Server) handleAdminCreateReservation(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.FormValue("user_id")
	productID := r.FormValue("product_id")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")
	price := r.FormValue("price")

	if userID == "" || productID == "" || startDate == "" || endDate == "" || price == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Generate order ID
	orderID := fmt.Sprintf("MANUAL-%d", time.Now().UnixNano())

	// Create order in read models
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
	INSERT INTO orders (id, user_id, total_amount, status, payment_method, start_date, end_date, created_at, updated_at)
	VALUES (?, ?, ?, 'confirmed', 'manual', ?, ?, ?, ?)
	`
	_, err := s.readModels.GetDB().Exec(query, orderID, userID, price, startDate, endDate, now, now)
	if err != nil {
		log.Printf("Failed to create manual reservation: %v", err)
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	// Block dates in product_bookings table
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		bookingQuery := `
		INSERT INTO product_bookings (product_id, order_id, booked_date)
		VALUES (?, ?, ?)
		`
		_, err = s.readModels.GetDB().Exec(bookingQuery, productID, orderID, dateStr)
		if err != nil {
			log.Printf("Failed to block date %s: %v", dateStr, err)
		}
	}

	// Return updated admin panel
	s.handleAdminPanel(w, r)
}

// handleAdminBlockProductDate blocks a date for a specific product (HTMX).
func (s *Server) handleAdminBlockProductDate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminProductDateAction(w, r, true)
}

// handleAdminUnblockProductDate unblocks a date for a specific product (HTMX).
func (s *Server) handleAdminUnblockProductDate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminProductDateAction(w, r, false)
}

// handleAdminProductDateAction handles blocking/unblocking dates for products.
func (s *Server) handleAdminProductDateAction(w http.ResponseWriter, r *http.Request, block bool) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	productID := chi.URLParam(r, "id")
	date := r.FormValue("date")

	if productID == "" || date == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	orderID := fmt.Sprintf("ADMIN-BLOCK-%s", productID)
	var err error

	if block {
		// Insert into product_bookings with a special order ID for admin blocks
		query := `
		INSERT OR IGNORE INTO product_bookings (product_id, order_id, booked_date)
		VALUES (?, ?, ?)
		`
		_, err = s.readModels.GetDB().Exec(query, productID, orderID, date)
		if err != nil {
			log.Printf("Failed to block product date: %v", err)
			http.Error(w, "Failed to block date", http.StatusInternalServerError)
			return
		}
	} else {
		// Delete from product_bookings for admin blocks
		query := `
		DELETE FROM product_bookings
		WHERE product_id = ? AND order_id = ? AND booked_date = ?
		`
		_, err = s.readModels.GetDB().Exec(query, productID, orderID, date)
		if err != nil {
			log.Printf("Failed to unblock product date: %v", err)
			http.Error(w, "Failed to unblock date", http.StatusInternalServerError)
			return
		}
	}

	// Return updated admin panel
	s.handleAdminPanel(w, r)
}

// handleHealth returns health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// Helper functions

// adminAuthMiddleware checks if user is authenticated as admin.
func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for session cookie
		cookie, err := r.Cookie("session_token")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Verify session and check if user is admin
		ctx := r.Context()
		_, isAdmin, err := s.authManager.VerifySession(ctx, cookie.Value)
		if err != nil || !isAdmin {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}

	if _, ok := data["IsLoggedIn"]; !ok {
		data["IsLoggedIn"] = isLoggedIn(r)
	}
	if _, ok := data["IsAdmin"]; !ok {
		data["IsAdmin"] = isAdmin(r)
	}
	if _, ok := data["CartCount"]; !ok {
		data["CartCount"] = getCartCount(r)
	}
	if _, ok := data["CartTotal"]; !ok {
		data["CartTotal"] = getCartTotal(r)
	}

	if _, isPartial := partialTemplates[name]; isPartial {
		if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("Template error (%s): %v", name, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	baseName := strings.TrimSuffix(name, ".html")
	contentName := baseName + "-content"

	var contentBuf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&contentBuf, contentName, data); err != nil {
		log.Printf("Template error (%s): %v", contentName, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	// #nosec G203 - content is already escaped by template engine
	data["Content"] = template.HTML(contentBuf.String())

	if err := s.templates.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("Template error (%s): %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func isLoggedIn(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	return err == nil && cookie.Value != ""
}

func isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	return err == nil && cookie.Value != ""
}

// Cart types and functions

type CartItem struct {
	CartID      string   `json:"cartId"`
	ProductID   string   `json:"productId"`
	ProductName string   `json:"productName"`
	BasePrice   int      `json:"basePrice"`
	StartDate   string   `json:"startDate"`
	EndDate     string   `json:"endDate"`
	RentalDays  int      `json:"rentalDays"`
	Addons      []string `json:"addons"`
	Total       int      `json:"total"`
}

func getCart(r *http.Request) []CartItem {
	if cookie, err := r.Cookie("cart"); err == nil {
		// URL-decode the cookie value to get the JSON
		decodedValue, err := url.QueryUnescape(cookie.Value)
		if err != nil {
			log.Printf("Error decoding cart cookie: %v", err)
			return []CartItem{}
		}
		var cart []CartItem
		if err := json.Unmarshal([]byte(decodedValue), &cart); err != nil {
			log.Printf("Error unmarshaling cart: %v", err)
			return []CartItem{}
		}
		return cart
	}
	return []CartItem{}
}

func setCart(w http.ResponseWriter, r *http.Request, cart []CartItem) {
	data, err := json.Marshal(cart)
	if err != nil {
		log.Printf("Error marshaling cart: %v", err)
		return
	}

	// URL-encode the JSON data to make it safe for cookies
	encodedValue := url.QueryEscape(string(data))

	// Only set Secure flag for non-localhost requests (for development)
	isLocalhost := r.Host == "localhost" || r.Host == "127.0.0.1" ||
		len(r.Host) >= 9 && r.Host[:9] == "localhost:" ||
		len(r.Host) >= 10 && r.Host[:10] == "127.0.0.1:"

	http.SetCookie(w, &http.Cookie{
		Name:     "cart",
		Value:    encodedValue,
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   !isLocalhost,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCart(w http.ResponseWriter, r *http.Request) {
	// Only set Secure flag for non-localhost requests (for development)
	isLocalhost := r.Host == "localhost" || r.Host == "127.0.0.1" ||
		len(r.Host) >= 9 && r.Host[:9] == "localhost:" ||
		len(r.Host) >= 10 && r.Host[:10] == "127.0.0.1:"

	http.SetCookie(w, &http.Cookie{
		Name:     "cart",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !isLocalhost,
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
