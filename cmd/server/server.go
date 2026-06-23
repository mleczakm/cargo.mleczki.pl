package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"cargo.mleczki.pl/internal/articles"
	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/email"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/middleware"
	"cargo.mleczki.pl/internal/notifications"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/transfers"
)

const methodPost = "POST"

// Server holds the application state.
type Server struct {
	eventStore      eventstore.EventStore
	readModels      *projections.ReadModelsDB
	projector       *projections.Projector
	productParser   *products.Parser
	articleParser   *articles.Parser
	authManager     *auth.AuthManager
	templates       *template.Template
	transferMatcher *transfers.Matcher
	emailImporter   *email.Importer
	adminNotifier   *notifications.AdminNotifier
	webPushNotifier *notifications.WebPushNotifier
	brevoClient     *email.BrevoClient
}

// partialTemplates are HTMX fragments rendered without the site layout.
var partialTemplates = map[string]struct{}{
	"calendar.html":            {},
	"payment-success-fragment": {},
}

// NewServer creates a new HTTP server.
func NewServer(eventStore eventstore.EventStore, readModels *projections.ReadModelsDB, projector *projections.Projector, productParser *products.Parser, articleParser *articles.Parser, authManager *auth.AuthManager) *Server {
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

	// Initialize admin notifier
	adminNotifier, err := notifications.NewAdminNotifier(readModels.GetDB())
	if err != nil {
		log.Printf("Failed to initialize admin notifier: %v", err)
		adminNotifier = nil
	} else {
		log.Println("Admin notifier initialized")
	}

	// Initialize web push notifier
	webPushNotifier, err := notifications.NewWebPushNotifier()
	if err != nil {
		log.Printf("Failed to initialize web push notifier: %v", err)
		webPushNotifier = nil
	} else {
		log.Println("Web push notifier initialized")
	}

	// Initialize Brevo client for password reset emails
	brevoClient, err := email.NewBrevoClient()
	if err != nil {
		log.Printf("Failed to initialize Brevo client: %v", err)
		brevoClient = nil
	} else {
		log.Println("Brevo client initialized")
	}

	server := &Server{
		eventStore:      eventStore,
		readModels:      readModels,
		projector:       projector,
		productParser:   productParser,
		articleParser:   articleParser,
		authManager:     authManager,
		templates:       tmpl,
		transferMatcher: transfers.NewMatcher(),
		emailImporter:   emailImporter,
		adminNotifier:   adminNotifier,
		webPushNotifier: webPushNotifier,
		brevoClient:     brevoClient,
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
	r.Use(middleware.SessionMiddleware(s.authManager))

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	r.Handle("/data/images/*", http.StripPrefix("/data/images/", http.FileServer(http.Dir("data/images"))))
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon-32.svg", http.StatusMovedPermanently)
	})

	// Public routes
	r.Get("/", s.handleHome)
	r.Get("/product/{id}", s.handleProduct)
	r.Get("/product/{id}/calendar", s.handleProductCalendar)
	r.Get("/articles", s.handleArticles)
	r.Get("/articles/porady", s.handleArticlesPorady)
	r.Get("/articles/recenzje", s.handleArticlesRecenzje)
	r.Get("/article/{id}", s.handleArticle)
	r.Get("/login", s.handleLogin)
	r.Post("/login", s.handleLogin)
	r.Get("/logout", s.handleLogout)
	r.Post("/logout", s.handleLogout)
	r.Get("/checkout", s.handleCheckout)
	r.Get("/payment", s.handlePayment)
	r.Get("/payment/{id}", s.handlePayment)
	r.Get("/success", s.handleSuccess)
	r.Get("/terms", s.handleTerms)
	r.Get("/forgot-password", s.handleForgotPassword)
	r.Post("/forgot-password", s.handleForgotPasswordSubmit)
	r.Get("/reset-password", s.handleResetPassword)
	r.Post("/reset-password", s.handleResetPasswordSubmit)

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
	r.Get("/user/order/{id}", s.handleUserOrderDetail)
	r.Post("/user/order/{id}/cancel", s.handleUserOrderCancel)
	r.Post("/user/delete-request", s.handleUserDeleteRequest)
	r.Post("/user/delete-confirm", s.handleUserDeleteConfirm)
	r.Post("/user/delete-cancel", s.handleUserDeleteCancel)
	r.Get("/user/profile", s.handleUserProfile)
	r.Post("/user/profile", s.handleUserProfileUpdate)
	r.Post("/user/change-password", s.handleUserChangePassword)

	// Admin panel (protected)
	r.Group(func(r chi.Router) {
		r.Use(s.adminAuthMiddleware)
		r.Get("/admin", s.handleAdminPanel)
		r.Get("/admin/users", s.handleAdminUsers)
		r.Get("/admin/user/{id}", s.handleAdminUserDetail)
		r.Post("/admin/user/{id}", s.handleAdminUserUpdate)
		r.Post("/admin/user/{id}/reset-password", s.handleAdminUserResetPassword)
		r.Post("/admin/order/{id}/mark-paid", s.handleAdminOrderMarkPaid)
		r.Post("/admin/order/{id}/confirm", s.handleAdminOrderConfirm)
		r.Post("/admin/transfer/{id}/link", s.handleAdminTransferLink)
		r.Post("/admin/transfers/import", s.handleAdminImportTransfers)
		r.Post("/admin/reservation/create", s.handleAdminCreateReservation)
		r.Post("/admin/product/{id}/block-date", s.handleAdminBlockProductDate)
		r.Post("/admin/product/{id}/unblock-date", s.handleAdminUnblockProductDate)
		r.Post("/admin/global-closure/add", s.handleAdminAddGlobalClosure)
		r.Post("/admin/global-closure/remove", s.handleAdminRemoveGlobalClosure)
		// Web push subscription endpoints
		r.Get("/api/webpush/vapid-key", s.handleWebPushVAPIDKey)
		r.Post("/api/webpush/subscribe", s.handleWebPushSubscribe)
		r.Post("/api/webpush/unsubscribe", s.handleWebPushUnsubscribe)
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

	// Filter products based on visibility
	filtered := filterProductsByVisibility(products, r)

	data := map[string]interface{}{
		"Title":     "Wynajem sprzętu rowerowego",
		"Products":  filtered,
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

	// Check visibility
	if !isItemVisible(product.Visibility, r) {
		http.NotFound(w, r)
		return
	}

	// Load related articles that reference this product
	relatedArticles, err := s.articleParser.LoadArticlesByProductID(productID)
	if err != nil {
		log.Printf("Error loading related articles: %v", err)
		relatedArticles = []domain.Article{}
	}

	now := time.Now()
	data := map[string]interface{}{
		"Title":           product.Name,
		"Product":         product,
		"RelatedArticles": relatedArticles,
		"CurrentMonth":    int(now.Month()),
		"CurrentYear":     now.Year(),
		"CartCount":       getCartCount(r),
		"CartTotal":       getCartTotal(r),
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

	dbBookedDates, err := s.readModels.GetBookedDatesForProduct(productID)
	if err != nil {
		log.Printf("Error loading booked dates for %s: %v", productID, err)
		dbBookedDates = []string{}
	}
	bookedDates := mergeDateLists(product.BookedDates, dbBookedDates)

	// Generate calendar grid
	calendarGrid := s.generateCalendarGrid(year, month, bookedDates, globalBlockedDates, startDate, endDate)

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
	if middleware.IsAuthenticated(r) {
		s.redirectAuthenticatedUser(w, r)
		return
	}

	errorMessage := ""
	if r.Method == methodPost {
		email := r.FormValue("email")
		password := r.FormValue("password")

		if email == "" || password == "" {
			errorMessage = "Adres e-mail i hasło są wymagane"
		} else {
			ctx := r.Context()
			sessionToken, user, err := s.authManager.Login(ctx, email, password)
			if err != nil {
				errorMessage = "Nieprawidłowy adres e-mail lub hasło"
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

				// Redirect based on user role or previous page
				s.setLoginRedirect(w, r, user.IsAdmin)
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

func (s *Server) redirectAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	target := loginRedirectTarget(r, middleware.IsAdmin(r))
	if r.URL.Query().Get("modal") == "1" || r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", target)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) setLoginRedirect(w http.ResponseWriter, r *http.Request, isAdmin bool) {
	w.Header().Set("HX-Redirect", loginRedirectTarget(r, isAdmin))
}

func loginRedirectTarget(r *http.Request, isAdmin bool) string {
	if target := safeRedirectPath(r.URL.Query().Get("next")); target != "" {
		return target
	}
	if target := safeRedirectPathFromReferer(r); target != "" {
		return target
	}
	if isAdmin {
		return "/admin"
	}
	return "/user"
}

func safeRedirectPathFromReferer(r *http.Request) string {
	referer := r.Header.Get("Referer")
	if referer == "" {
		return ""
	}

	refURL, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	if refURL.Host != "" && refURL.Host != r.Host {
		return ""
	}

	path := refURL.Path
	if refURL.RawQuery != "" {
		path += "?" + refURL.RawQuery
	}
	return safeRedirectPath(path)
}

func safeRedirectPath(path string) string {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return ""
	}

	lower := strings.ToLower(path)
	switch {
	case strings.HasPrefix(lower, "/login"):
		return ""
	case strings.HasPrefix(lower, "/logout"):
		return ""
	}

	return path
}

// getBaseURL returns the base URL of the current request (scheme + host).
func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	return fmt.Sprintf("%s://%s", scheme, host)
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
	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		orderID = r.URL.Query().Get("id")
	}
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

	if paymentMethod == domain.PaymentMethodCashPickup || paymentMethod == domain.PaymentMethodBlikPickup {
		http.Redirect(w, r, fmt.Sprintf("/user/order/%s", orderID), http.StatusFound)
		return
	}

	data := map[string]interface{}{
		"Title":             "Płatność",
		"PaymentMethod":     paymentMethod,
		"FinalTotal":        int(totalAmount),
		"OrderID":           orderID,
		"PaymentCode":       paymentCode,
		"ShowConfirmButton": false,
		"ShowPolling":       paymentMethod == domain.PaymentMethodBlik && status == string(domain.StatusAwaitingPayment),
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

// validateCheckoutForm validates the checkout form data.
func validateCheckoutForm(r *http.Request) (string, string, string, string, string, error) {
	paymentMethod := r.FormValue("payment_method")
	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	isAdult := r.FormValue("is_adult") == "on"
	acceptTOS := r.FormValue("accept_tos") == "on"

	if name == "" || email == "" || phone == "" || address == "" || !isAdult || !acceptTOS {
		return "", "", "", "", "", fmt.Errorf("missing required fields")
	}

	return paymentMethod, name, email, phone, address, nil
}

// getOrCreateUser gets existing user or creates a new one.
func (s *Server) getOrCreateUser(ctx context.Context, email, name, phone, address, passwordHash string) (string, error) {
	var userID string
	err := s.readModels.GetDB().QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", email).Scan(&userID)
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.readModels.GetDB().ExecContext(ctx, `
			UPDATE users SET name = ?, phone = ?, address = ?, updated_at = ?
			WHERE id = ?
		`, name, phone, address, now, userID)
		if err != nil {
			log.Printf("Failed to update user details: %v", err)
		}
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to lookup user: %w", err)
	}

	userID = fmt.Sprintf("user_%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.readModels.GetDB().ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, email, passwordHash, name, phone, address, 1, 1, 0, now, now)
	if err != nil {
		return "", fmt.Errorf("failed to create user: %w", err)
	}

	return userID, nil
}

// convertCartToOrderItems converts cart items to domain.OrderItem format.
func (s *Server) convertCartToOrderItems(cart []CartItem) []domain.OrderItem {
	var orderItems []domain.OrderItem
	for _, item := range cart {
		product, err := s.productParser.LoadProductByID(item.ProductID)
		if err != nil {
			log.Printf("Failed to load product %s: %v", item.ProductID, err)
			continue
		}

		var selectedAddons []domain.Addon
		for _, addonID := range item.Addons {
			for _, productAddon := range product.Addons {
				if productAddon.ID == addonID {
					selectedAddons = append(selectedAddons, domain.Addon{
						ID:    productAddon.ID,
						Name:  productAddon.Name,
						Price: productAddon.Price,
					})
					break
				}
			}
		}

		orderItems = append(orderItems, domain.OrderItem{
			ProductID:      item.ProductID,
			ProductName:    item.ProductName,
			BasePrice:      item.BasePrice,
			SelectedAddons: selectedAddons,
			RentalDays:     item.RentalDays,
		})
	}

	return orderItems
}

// mergeDateLists returns a deduplicated merge of date lists.
func mergeDateLists(lists ...[]string) []string {
	seen := make(map[string]struct{})
	var merged []string
	for _, list := range lists {
		for _, date := range list {
			if _, ok := seen[date]; ok {
				continue
			}
			seen[date] = struct{}{}
			merged = append(merged, date)
		}
	}
	return merged
}

// validateCartAvailability ensures cart items can still be booked.
func (s *Server) validateCartAvailability(cart []CartItem) error {
	globalBlockedDates, err := s.readModels.GetGlobalBlockedDates()
	if err != nil {
		return fmt.Errorf("failed to check availability: %w", err)
	}

	for _, item := range cart {
		product, err := s.productParser.LoadProductByID(item.ProductID)
		if err != nil {
			return fmt.Errorf("product not found: %s", item.ProductID)
		}

		start, err := time.Parse("2006-01-02", item.StartDate)
		if err != nil {
			return fmt.Errorf("invalid start date for %s", item.ProductName)
		}
		end, err := time.Parse("2006-01-02", item.EndDate)
		if err != nil {
			return fmt.Errorf("invalid end date for %s", item.ProductName)
		}

		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			if s.isDateBooked(dateStr, product.BookedDates, globalBlockedDates) {
				return fmt.Errorf("selected dates are no longer available")
			}

			booked, err := s.readModels.IsProductDateBooked(item.ProductID, dateStr)
			if err != nil {
				return fmt.Errorf("failed to check availability: %w", err)
			}
			if booked {
				return fmt.Errorf("selected dates are no longer available")
			}
		}
	}

	return nil
}

func hashOptionalPassword(r *http.Request) (string, error) {
	password := r.FormValue("password")
	if password == "" {
		return "", nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *Server) buildOrderPlacedEvent(
	orderID, userID string,
	cart []CartItem,
	totalAmount int,
	paymentMethod string,
	paymentCode *string,
	isFirstOrder bool,
) *domain.OrderPlacedEvent {
	var startDate, endDate string
	var rentalDays int
	if len(cart) > 0 {
		startDate = cart[0].StartDate
		endDate = cart[0].EndDate
		rentalDays = cart[0].RentalDays
	}

	return &domain.OrderPlacedEvent{
		OrderID:       orderID,
		UserID:        userID,
		Items:         s.convertCartToOrderItems(cart),
		TotalAmount:   totalAmount,
		PaymentMethod: paymentMethod,
		PaymentCode:   paymentCode,
		StartDate:     startDate,
		EndDate:       endDate,
		RentalDays:    rentalDays,
		IsFirstOrder:  isFirstOrder,
		Timestamp:     time.Now().UTC(),
	}
}

func (s *Server) persistPlacedOrder(ctx context.Context, orderID string, orderEvent *domain.OrderPlacedEvent) error {
	eventData, err := eventstore.ToEvent(orderID, "order", orderEvent, 1)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	if err := s.eventStore.Save(ctx, eventData); err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	if err := s.projector.Run(ctx); err != nil {
		return fmt.Errorf("project event: %w", err)
	}
	exists, err := s.readModels.OrderExists(orderID)
	if err != nil {
		return fmt.Errorf("check order exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("order %s missing after projection", orderID)
	}
	return nil
}

func (s *Server) saveBLIKPaymentCode(orderID string, paymentCode *string) {
	if paymentCode == nil {
		return
	}
	paymentCodeID := fmt.Sprintf("pc_%d", time.Now().UnixNano())
	if err := s.readModels.CreatePaymentCode(paymentCodeID, *paymentCode, orderID); err != nil {
		log.Printf("Failed to create payment code: %v", err)
	}
}

func (s *Server) notifyPickupOrderAdminsAsync(parent context.Context, orderID, name, email, paymentMethod string, totalAmount int) {
	if !isPickupPaymentMethod(paymentMethod) {
		return
	}

	notifyCtx := context.WithoutCancel(parent)
	if s.adminNotifier != nil {
		go func(ctx context.Context) {
			if err := s.adminNotifier.NotifyOrderRequiringConfirmation(ctx, orderID, name, email, paymentMethod, float64(totalAmount)); err != nil {
				log.Printf("Failed to send admin email notification for order %s: %v", orderID, err)
			}
		}(notifyCtx)
	}
	if s.webPushNotifier != nil {
		go func(ctx context.Context) {
			if err := s.webPushNotifier.NotifyOrderRequiringConfirmation(ctx, orderID, name, paymentMethod, float64(totalAmount)); err != nil {
				log.Printf("Failed to send admin web push notification for order %s: %v", orderID, err)
			}
		}(notifyCtx)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Failed to write JSON response: %v", err)
	}
}

// handleCheckoutSubmit processes the checkout form (HTMX).
func (s *Server) handleCheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate form data
	paymentMethod, name, email, phone, address, err := validateCheckoutForm(r)
	if err != nil {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Get cart
	cart := getCart(r)
	if len(cart) == 0 {
		http.Error(w, "Cart is empty", http.StatusBadRequest)
		return
	}

	if err := s.validateCartAvailability(cart); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Calculate total
	totalAmount := calculateFinalTotal(cart)

	// Generate order ID
	orderID := fmt.Sprintf("ORD-%d", time.Now().UnixNano())

	// Generate payment code for BLIK payments
	var paymentCode *string
	if paymentMethod == domain.PaymentMethodBlik {
		code := domain.GeneratePaymentCode()
		paymentCode = &code
	}

	passwordHash, err := hashOptionalPassword(r)
	if err != nil {
		http.Error(w, "Failed to process password", http.StatusInternalServerError)
		return
	}

	// Get or create user
	ctx := r.Context()
	userID, err := s.getOrCreateUser(ctx, email, name, phone, address, passwordHash)
	if err != nil {
		http.Error(w, "Failed to create/get user", http.StatusInternalServerError)
		return
	}

	// Check if this is the user's first order
	isFirstOrder, err := s.readModels.IsFirstOrder(userID)
	if err != nil {
		log.Printf("Failed to check if first order: %v", err)
		isFirstOrder = true // Default to requiring confirmation on error
	}

	orderEvent := s.buildOrderPlacedEvent(orderID, userID, cart, totalAmount, paymentMethod, paymentCode, isFirstOrder)
	if err := s.persistPlacedOrder(ctx, orderID, orderEvent); err != nil {
		log.Printf("Failed to create order: %v", err)
		http.Error(w, "Failed to create order", http.StatusInternalServerError)
		return
	}

	if paymentMethod == domain.PaymentMethodBlik {
		s.saveBLIKPaymentCode(orderID, paymentCode)
	}

	s.notifyPickupOrderAdminsAsync(ctx, orderID, name, email, paymentMethod, totalAmount)

	// Clear cart
	clearCart(w, r)

	if isPickupPaymentMethod(paymentMethod) {
		http.Redirect(w, r, fmt.Sprintf("/user/order/%s", orderID), http.StatusFound)
		return
	}

	// Redirect to payment
	redirectURL := fmt.Sprintf("/payment/%s?method=%s", orderID, paymentMethod)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handlePaymentConfirm confirms payment (HTMX). Pickup orders are confirmed by admin only.
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

	var paymentMethod string
	err := s.readModels.GetDB().QueryRow(`SELECT payment_method FROM orders WHERE id = ?`, orderID).Scan(&paymentMethod)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}
	if paymentMethod == domain.PaymentMethodCashPickup || paymentMethod == domain.PaymentMethodBlikPickup {
		http.Error(w, "Pickup orders are confirmed by the shop", http.StatusBadRequest)
		return
	}

	// Emit OrderPaidEvent for cash payment confirmation
	event := &domain.OrderPaidEvent{
		OrderID:   orderID,
		Method:    "cash",
		Timestamp: time.Now().UTC(),
	}

	eventData, err := eventstore.ToEvent(orderID, "order", event, 0)
	if err != nil {
		log.Printf("Failed to create OrderPaidEvent: %v", err)
		http.Error(w, "Failed to confirm order", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := s.eventStore.Save(ctx, eventData); err != nil {
		log.Printf("Failed to emit OrderPaidEvent: %v", err)
		http.Error(w, "Failed to confirm order", http.StatusInternalServerError)
		return
	}

	// Clear cart
	clearCart(w, r)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/success")
		return
	}
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

	// If order is confirmed, return success fragment for HTMX polling.
	if status == string(domain.StatusConfirmed) {
		s.renderPartialTemplate(w, "payment-success-fragment", map[string]interface{}{
			"Email": "user@example.com",
		})
		return
	}

	// Otherwise, return loading state and keep polling.
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div id="payment-status" hx-get="/payment/status/%s" hx-trigger="every 3s" hx-target="#payment-status" hx-swap="outerHTML">
		<div class="flex flex-col items-center justify-center py-4">
			<svg class="w-10 h-10 text-emerald-600 animate-spin mb-4" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
			</svg>
			<p class="font-bold text-gray-900">Nasłuchujemy na przelew z banku...</p>
		</div>
	</div>`, orderID)
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
	// Get user from context
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Fetch user's orders from read models
	orders, err := s.readModels.GetOrdersByUserID(authenticatedUser.ID)
	if err != nil {
		log.Printf("Failed to fetch user orders: %v", err)
		orders = []map[string]interface{}{}
	}

	enrichedOrders := make([]map[string]interface{}, 0, len(orders))
	for _, order := range orders {
		enrichedOrders = append(enrichedOrders, enrichOrderSummary(order))
	}

	data := map[string]interface{}{
		"Title":       "Moje zamówienia",
		"User":        authenticatedUser,
		"Orders":      enrichedOrders,
		"DeleteState": "idle",
	}

	data["IsLoggedIn"] = true
	s.renderTemplate(w, r, "user_panel.html", data)
}

// handleUserOrderDetail renders a single order with payment instructions.
func (s *Server) handleUserOrderDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	orderID := chi.URLParam(r, "id")
	order, err := s.readModels.GetOrderByIDAndUserID(orderID, authenticatedUser.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("Failed to fetch order %s: %v", orderID, err)
		http.Error(w, "Failed to load order", http.StatusInternalServerError)
		return
	}

	view := buildOrderDetailView(order)
	data := map[string]interface{}{
		"Title":                "Szczegóły zamówienia",
		"Order":                view.Order,
		"ParsedItems":          view.ParsedItems,
		"StatusLabel":          view.StatusLabel,
		"StatusBadgeClass":     view.StatusBadgeClass,
		"PaymentMethodLabel":   view.PaymentMethodLabel,
		"NeedsPayment":         view.NeedsPayment,
		"AwaitingConfirmation": view.AwaitingConfirmation,
		"CanCancel":            view.CanCancel,
		"PaymentView":          view.PaymentView,
		"IsLoggedIn":           true,
	}

	s.renderTemplate(w, r, "order_detail.html", data)
}

// handleUserOrderCancel cancels an order owned by the current user.
func (s *Server) handleUserOrderCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	orderID := chi.URLParam(r, "id")
	order, err := s.readModels.GetOrderByIDAndUserID(orderID, authenticatedUser.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Failed to load order", http.StatusInternalServerError)
		return
	}

	status, _ := order["Status"].(string)
	paymentMethod, _ := order["PaymentMethod"].(string)
	if !canUserCancelOrder(status, paymentMethod) {
		http.Error(w, "Order cannot be cancelled", http.StatusBadRequest)
		return
	}

	event := &domain.OrderCancelledEvent{
		OrderID:   orderID,
		Timestamp: time.Now().UTC(),
	}
	eventData, err := eventstore.ToEvent(orderID, "order", event, 0)
	if err != nil {
		http.Error(w, "Failed to cancel order", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := s.eventStore.Save(ctx, eventData); err != nil {
		log.Printf("Failed to emit OrderCancelledEvent: %v", err)
		http.Error(w, "Failed to cancel order", http.StatusInternalServerError)
		return
	}
	if err := s.projector.Run(ctx); err != nil {
		log.Printf("Failed to project order cancellation: %v", err)
		http.Error(w, "Failed to cancel order", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/user/order/%s", orderID), http.StatusFound)
}

type orderDetailView struct {
	Order                map[string]interface{}
	ParsedItems          []domain.OrderItem
	StatusLabel          string
	StatusBadgeClass     string
	PaymentMethodLabel   string
	NeedsPayment         bool
	AwaitingConfirmation bool
	CanCancel            bool
	PaymentView          map[string]interface{}
}

func buildOrderDetailView(order map[string]interface{}) orderDetailView {
	enriched := enrichOrderSummary(order)
	status, _ := enriched["Status"].(string)
	paymentMethod, _ := enriched["PaymentMethod"].(string)
	orderID, _ := enriched["ID"].(string)
	totalAmount, _ := enriched["TotalAmount"].(float64)

	paymentCode := ""
	if code, ok := enriched["PaymentCode"].(string); ok {
		paymentCode = code
	}

	itemsJSON, _ := enriched["Items"].(string)

	needsPayment := status == string(domain.StatusAwaitingPayment)
	awaitingConfirmation := status == string(domain.StatusPaid) && isPickupPaymentMethod(paymentMethod)
	canCancel := canUserCancelOrder(status, paymentMethod)
	paymentView := map[string]interface{}{
		"OrderID":           orderID,
		"FinalTotal":        int(totalAmount),
		"ShowConfirmButton": false,
		"ShowPolling":       needsPayment && paymentMethod == domain.PaymentMethodBlik,
	}
	if paymentCode != "" {
		paymentView["PaymentCode"] = paymentCode
	}

	return orderDetailView{
		Order:                enriched,
		ParsedItems:          parseOrderItems(itemsJSON),
		StatusLabel:          orderStatusLabel(status, paymentMethod),
		StatusBadgeClass:     orderStatusBadgeClass(status, paymentMethod),
		PaymentMethodLabel:   paymentMethodLabel(paymentMethod, status),
		NeedsPayment:         needsPayment,
		AwaitingConfirmation: awaitingConfirmation,
		CanCancel:            canCancel,
		PaymentView:          paymentView,
	}
}

func enrichOrderSummary(order map[string]interface{}) map[string]interface{} {
	enriched := make(map[string]interface{}, len(order)+8)
	for k, v := range order {
		enriched[k] = v
	}

	itemsJSON, _ := order["Items"].(string)
	items := parseOrderItems(itemsJSON)
	enriched["ParsedItems"] = items
	if len(items) > 0 {
		enriched["PrimaryItemName"] = items[0].ProductName
	}
	enriched["ItemCount"] = len(items)
	if len(items) > 1 {
		enriched["ExtraItemCount"] = len(items) - 1
	}

	status, _ := order["Status"].(string)
	paymentMethod, _ := order["PaymentMethod"].(string)
	enriched["StatusLabel"] = orderStatusLabel(status, paymentMethod)
	enriched["StatusBadgeClass"] = orderStatusBadgeClass(status, paymentMethod)
	enriched["PaymentMethodLabel"] = paymentMethodLabel(paymentMethod, status)
	enriched["NeedsPayment"] = status == string(domain.StatusAwaitingPayment)
	enriched["AwaitingConfirmation"] = status == string(domain.StatusPaid) && isPickupPaymentMethod(paymentMethod)
	enriched["CanCancel"] = canUserCancelOrder(status, paymentMethod)

	return enriched
}

func isPickupPaymentMethod(method string) bool {
	return method == domain.PaymentMethodCashPickup || method == domain.PaymentMethodBlikPickup
}

func canUserCancelOrder(status, paymentMethod string) bool {
	switch status {
	case string(domain.StatusCancelled), string(domain.StatusConfirmed), string(domain.StatusRealized):
		return false
	case string(domain.StatusAwaitingPayment):
		return true
	case string(domain.StatusPaid):
		return isPickupPaymentMethod(paymentMethod)
	default:
		return false
	}
}

func parseOrderItems(itemsJSON string) []domain.OrderItem {
	if itemsJSON == "" {
		return nil
	}
	var items []domain.OrderItem
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		log.Printf("Failed to parse order items: %v", err)
		return nil
	}
	return items
}

func orderStatusLabel(status, paymentMethod string) string {
	switch status {
	case string(domain.StatusAwaitingPayment):
		return "Oczekuje na płatność"
	case string(domain.StatusPaid):
		if isPickupPaymentMethod(paymentMethod) {
			return "Oczekuje na potwierdzenie"
		}
		return "Opłacone"
	case string(domain.StatusConfirmed):
		return "Potwierdzone"
	case string(domain.StatusCancelled):
		return "Anulowane"
	case string(domain.StatusRealized):
		return "Zrealizowane"
	default:
		return status
	}
}

func orderStatusBadgeClass(status, paymentMethod string) string {
	switch status {
	case string(domain.StatusAwaitingPayment):
		return "bg-yellow-100 text-yellow-800"
	case string(domain.StatusPaid):
		if isPickupPaymentMethod(paymentMethod) {
			return "bg-amber-100 text-amber-800"
		}
		return "bg-emerald-100 text-emerald-800"
	case string(domain.StatusConfirmed), string(domain.StatusRealized):
		return "bg-emerald-100 text-emerald-800"
	case string(domain.StatusCancelled):
		return "bg-red-100 text-red-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}

func paymentMethodLabel(method, status string) string {
	switch method {
	case domain.PaymentMethodBlik:
		if status == string(domain.StatusAwaitingPayment) {
			return "BLIK na telefon"
		}
		return "Opłacone (BLIK)"
	case domain.PaymentMethodCashPickup:
		if status == string(domain.StatusAwaitingPayment) {
			return "Gotówka przy odbiorze"
		}
		return "Gotówka przy odbiorze"
	case domain.PaymentMethodBlikPickup:
		return "BLIK przy odbiorze"
	default:
		return method
	}
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

// handleForgotPassword renders the forgot password form.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("modal") != "1" {
		http.Redirect(w, r, "/?auth=forgot-password", http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Title": "Przypomnij hasło",
	}
	if err := s.templates.ExecuteTemplate(w, "forgot_password-content", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleForgotPasswordSubmit processes the forgot password form.
func (s *Server) handleForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userEmail := r.FormValue("email")
	if userEmail == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	token, err := s.authManager.RequestPasswordReset(ctx, userEmail)
	if err != nil {
		log.Printf("Failed to request password reset: %v", err)
		http.Error(w, "Failed to request password reset", http.StatusInternalServerError)
		return
	}

	// Send email with reset link if Brevo is configured
	if token != "" && s.brevoClient != nil {
		resetLink := fmt.Sprintf("%s/reset-password?token=%s", getBaseURL(r), token)

		htmlContent := fmt.Sprintf(`
			<h2>Reset your password</h2>
			<p>Hello,</p>
			<p>We received a request to reset your password for your account at cargo.mleczki.pl.</p>
			<p>Click the link below to reset your password:</p>
			<p><a href="%s" style="background-color: #007bff; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px;">Reset Password</a></p>
			<p>Or copy and paste this link into your browser:</p>
			<p>%s</p>
			<p>This link will expire in 1 hour.</p>
			<p>If you did not request this password reset, please ignore this email.</p>
		`, resetLink, resetLink)

		sender := &email.EmailSender{Name: "Cargo Mleczki", Email: "noreply@cargo.mleczki.pl"}
		to := []email.EmailRecipient{{Email: userEmail}}
		err = s.brevoClient.SendEmail(ctx, sender, to, "Reset your password", htmlContent)

		if err != nil {
			log.Printf("Failed to send password reset email: %v", err)
			// Fall through to show success message anyway (security)
		}
	} else if token != "" {
		// Log the token for testing if Brevo is not configured
		log.Printf("Password reset token for %s: %s", userEmail, token)
		resetLink := fmt.Sprintf("/reset-password?token=%s", token)
		data := map[string]interface{}{
			"Title":     "Przypomnij hasło",
			"Email":     userEmail,
			"ResetLink": resetLink,
			"TokenSent": true,
		}
		s.renderForgotPassword(w, data)
		return
	}

	// Always show success message even if user doesn't exist (security)
	data := map[string]interface{}{
		"Title":     "Przypomnij hasło",
		"EmailSent": true,
	}
	s.renderForgotPassword(w, data)
}

func (s *Server) renderForgotPassword(w http.ResponseWriter, data map[string]interface{}) {
	if err := s.templates.ExecuteTemplate(w, "forgot_password-content", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleResetPassword renders the reset password form.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	data := map[string]interface{}{
		"Title": "Resetuj hasło",
		"Token": token,
	}
	s.renderTemplate(w, r, "reset_password.html", data)
}

// handleResetPasswordSubmit processes the reset password form.
func (s *Server) handleResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.FormValue("token")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if token == "" || newPassword == "" || confirmPassword == "" {
		data := map[string]interface{}{
			"Title":        "Resetuj hasło",
			"Token":        token,
			"ErrorMessage": "Wszystkie pola są wymagane",
		}
		s.renderTemplate(w, r, "reset_password.html", data)
		return
	}

	if newPassword != confirmPassword {
		data := map[string]interface{}{
			"Title":        "Resetuj hasło",
			"Token":        token,
			"ErrorMessage": "Hasła nie są zgodne",
		}
		s.renderTemplate(w, r, "reset_password.html", data)
		return
	}

	if len(newPassword) < 8 {
		data := map[string]interface{}{
			"Title":        "Resetuj hasło",
			"Token":        token,
			"ErrorMessage": "Hasło musi mieć minimum 8 znaków",
		}
		s.renderTemplate(w, r, "reset_password.html", data)
		return
	}

	ctx := r.Context()
	err := s.authManager.ResetPassword(ctx, token, newPassword)
	if err != nil {
		log.Printf("Failed to reset password: %v", err)
		data := map[string]interface{}{
			"Title":        "Resetuj hasło",
			"Token":        token,
			"ErrorMessage": err.Error(),
		}
		s.renderTemplate(w, r, "reset_password.html", data)
		return
	}

	// Show success page
	data := map[string]interface{}{
		"Title":   "Resetuj hasło",
		"Success": true,
	}
	s.renderTemplate(w, r, "reset_password.html", data)
}

// handleUserProfile renders the user profile form.
func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	data := map[string]interface{}{
		"Title": "Edytuj profil",
		"User":  authenticatedUser,
	}
	data["IsLoggedIn"] = true
	s.renderTemplate(w, r, "user_profile.html", data)
}

// handleUserProfileUpdate processes the user profile update form.
func (s *Server) handleUserProfileUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")
	address := r.FormValue("address")

	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	err := s.authManager.UpdateUserProfile(ctx, authenticatedUser.ID, name, phone, address)
	if err != nil {
		log.Printf("Failed to update user profile: %v", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/user", http.StatusFound)
}

// handleUserChangePassword processes the password change form.
func (s *Server) handleUserChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	authenticatedUser, ok := user.(*domain.User)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	if len(newPassword) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	err := s.authManager.ChangePassword(ctx, authenticatedUser.ID, currentPassword, newPassword)
	if err != nil {
		log.Printf("Failed to change password: %v", err)
		http.Error(w, "Failed to change password: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Clear session cookie
	clearCart(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// handleAdminUsers renders the admin users list.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := s.authManager.GetAllUsers(ctx)
	if err != nil {
		log.Printf("Failed to fetch users: %v", err)
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title": "Użytkownicy",
		"Users": users,
	}
	data["IsAdmin"] = true
	s.renderTemplate(w, r, "admin_users.html", data)
}

// handleAdminUserUpdate processes the admin user update form.
func (s *Server) handleAdminUserUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := chi.URLParam(r, "id")
	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	isAdmin := r.FormValue("is_admin") == "on"

	if userID == "" || name == "" || email == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	err := s.authManager.AdminUpdateUser(ctx, userID, name, email, phone, address, isAdmin)
	if err != nil {
		log.Printf("Failed to update user: %v", err)
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

// handleAdminUserResetPassword resets a user's password (admin).
func (s *Server) handleAdminUserResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := chi.URLParam(r, "id")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	newPassword, err := s.authManager.AdminResetPassword(ctx, userID)
	if err != nil {
		log.Printf("Failed to reset password: %v", err)
		http.Error(w, "Failed to reset password", http.StatusInternalServerError)
		return
	}

	// Log the new password (in production, this should be sent via email)
	log.Printf("Reset password for user %s. New password: %s", userID, newPassword)

	http.Redirect(w, r, "/admin/users", http.StatusFound)
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

	// Fetch orders awaiting confirmation (paid pickup payments)
	ordersAwaitingConfirmation, err := s.readModels.GetOrdersAwaitingConfirmation()
	if err != nil {
		log.Printf("Error loading orders awaiting confirmation: %v", err)
		ordersAwaitingConfirmation = []map[string]interface{}{}
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
		"Title":                      "Panel Administratora",
		"Orders":                     orders,
		"OrdersAwaitingConfirmation": ordersAwaitingConfirmation,
		"Transfers":                  transfers,
		"Users":                      users,
		"Products":                   products,
		"GlobalBlockedDates":         globalBlockedDates,
		"LastEmailImport":            lastEmailImport,
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

// emitOrderPaidEvent is a helper function to emit OrderPaidEvent.
func (s *Server) emitOrderPaidEvent(w http.ResponseWriter, r *http.Request, orderID, method string) {
	event := &domain.OrderPaidEvent{
		OrderID:   orderID,
		Method:    method,
		Timestamp: time.Now().UTC(),
	}

	eventData, err := eventstore.ToEvent(orderID, "order", event, 0)
	if err != nil {
		log.Printf("Failed to create OrderPaidEvent: %v", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := s.eventStore.Save(ctx, eventData); err != nil {
		log.Printf("Failed to emit OrderPaidEvent: %v", err)
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}
}

// handleAdminOrderMarkPaid marks an order as paid (HTMX).
func (s *Server) handleAdminOrderMarkPaid(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	s.emitOrderPaidEvent(w, r, orderID, "admin_manual")
	s.handleAdminPanel(w, r)
}

// handleAdminOrderConfirm confirms an order manually (HTMX).
func (s *Server) handleAdminOrderConfirm(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	// Emit OrderConfirmedEvent
	event := &domain.OrderConfirmedEvent{
		OrderID:   orderID,
		Timestamp: time.Now().UTC(),
	}

	eventData, err := eventstore.ToEvent(orderID, "order", event, 0)
	if err != nil {
		log.Printf("Failed to create OrderConfirmedEvent: %v", err)
		http.Error(w, "Failed to confirm order", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := s.eventStore.Save(ctx, eventData); err != nil {
		log.Printf("Failed to emit OrderConfirmedEvent: %v", err)
		http.Error(w, "Failed to confirm order", http.StatusInternalServerError)
		return
	}

	s.handleAdminPanel(w, r)
}

// handleWebPushVAPIDKey returns the VAPID public key for web push subscriptions.
func (s *Server) handleWebPushVAPIDKey(w http.ResponseWriter, r *http.Request) {
	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{
		"publicKey": s.webPushNotifier.GetVAPIDPublicKey(),
	})
}

// handleWebPushSubscribe registers a web push subscription.
func (s *Server) handleWebPushSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	var sub webpush.Subscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "Invalid subscription", http.StatusBadRequest)
		return
	}

	// Get user ID from session or context
	userID := "admin" // In production, get from authenticated user session

	s.webPushNotifier.AddSubscription(userID, sub)

	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{
		"status": "subscribed",
	})
}

// handleWebPushUnsubscribe removes a web push subscription.
func (s *Server) handleWebPushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	// Get user ID from session or context
	userID := "admin" // In production, get from authenticated user session

	s.webPushNotifier.RemoveSubscription(userID)

	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{
		"status": "unsubscribed",
	})
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

	ctx := r.Context()
	var matchedOrderID string

	// If order ID is provided manually, use it
	if orderID != "" {
		matchedOrderID = orderID
	} else {
		// Automatic matching by payment code
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
				err = s.readModels.GetDB().QueryRow(`
					SELECT order_id FROM payment_codes WHERE code = ?
				`, matchedCode).Scan(&matchedOrderID)
				if err != nil {
					log.Printf("Failed to get order by payment code: %v", err)
				}
			}
		}
	}

	// If we found a matching order, emit events
	if matchedOrderID != "" {
		// Emit TransferLinkedEvent
		transferEvent := &domain.TransferLinkedEvent{
			TransferID: transferID,
			OrderID:    matchedOrderID,
			Timestamp:  time.Now().UTC(),
		}

		transferEventData, err := eventstore.ToEvent(transferID, "transfer", transferEvent, 0)
		if err != nil {
			log.Printf("Failed to create TransferLinkedEvent: %v", err)
		} else {
			if err := s.eventStore.Save(ctx, transferEventData); err != nil {
				log.Printf("Failed to emit TransferLinkedEvent: %v", err)
			}
		}

		// Emit OrderPaidEvent
		orderEvent := &domain.OrderPaidEvent{
			OrderID:   matchedOrderID,
			Method:    "transfer",
			Timestamp: time.Now().UTC(),
		}

		orderEventData, err := eventstore.ToEvent(matchedOrderID, "order", orderEvent, 0)
		if err != nil {
			log.Printf("Failed to create OrderPaidEvent: %v", err)
		} else {
			if err := s.eventStore.Save(ctx, orderEventData); err != nil {
				log.Printf("Failed to emit OrderPaidEvent: %v", err)
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

// handleAdminAddGlobalClosure adds a global store closure date (HTMX).
func (s *Server) handleAdminAddGlobalClosure(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	date := r.FormValue("date")
	if date == "" {
		http.Error(w, "Missing required field: date", http.StatusBadRequest)
		return
	}

	// Get current user ID for audit trail
	userID := ""
	if user := middleware.GetUser(r); user != nil {
		if u, ok := user.(*domain.User); ok {
			userID = u.ID
		}
	}

	err := s.readModels.AddGlobalBlockedDate(date, userID)
	if err != nil {
		log.Printf("Failed to add global closure date: %v", err)
		http.Error(w, "Failed to add closure date", http.StatusInternalServerError)
		return
	}

	// Return updated admin panel
	s.handleAdminPanel(w, r)
}

// handleAdminRemoveGlobalClosure removes a global store closure date (HTMX).
func (s *Server) handleAdminRemoveGlobalClosure(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	date := r.FormValue("date")
	if date == "" {
		http.Error(w, "Missing required field: date", http.StatusBadRequest)
		return
	}

	err := s.readModels.RemoveGlobalBlockedDate(date)
	if err != nil {
		log.Printf("Failed to remove global closure date: %v", err)
		http.Error(w, "Failed to remove closure date", http.StatusInternalServerError)
		return
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
		s.renderPartialTemplate(w, name, data)
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

func (s *Server) renderPartialTemplate(w http.ResponseWriter, name string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error (%s): %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func isLoggedIn(r *http.Request) bool {
	return middleware.IsAuthenticated(r)
}

func isAdmin(r *http.Request) bool {
	return middleware.IsAdmin(r)
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

// isItemVisible checks if an item (product or article) is visible based on its visibility setting.
func isItemVisible(visibility domain.Visibility, r *http.Request) bool {
	switch visibility {
	case domain.VisibilityPublic:
		return true
	case domain.VisibilityLoggedIn:
		return isLoggedIn(r)
	case domain.VisibilityAdmin:
		return isAdmin(r)
	case domain.VisibilityHidden:
		return false
	default:
		return true // Default to public if unknown
	}
}

// filterArticlesByVisibility filters articles based on visibility settings.
func filterArticlesByVisibility(articles []domain.Article, r *http.Request) []domain.Article {
	var filtered []domain.Article
	for _, article := range articles {
		if isItemVisible(article.Visibility, r) {
			filtered = append(filtered, article)
		}
	}
	return filtered
}

// filterProductsByVisibility filters products based on visibility settings.
func filterProductsByVisibility(products []domain.Product, r *http.Request) []domain.Product {
	var filtered []domain.Product
	for _, product := range products {
		if isItemVisible(product.Visibility, r) {
			filtered = append(filtered, product)
		}
	}
	return filtered
}

// handleArticles renders the articles list page.
func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	articles, err := s.articleParser.LoadAllArticles()
	if err != nil {
		log.Printf("Error loading articles: %v", err)
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		return
	}

	// Filter articles based on visibility
	filtered := filterArticlesByVisibility(articles, r)

	data := map[string]interface{}{
		"Title":     "Artykuły",
		"Articles":  filtered,
		"CartCount": getCartCount(r),
		"CartTotal": getCartTotal(r),
	}

	s.renderTemplate(w, r, "articles.html", data)
}

// handleArticlesPorady renders the porady (tips) articles page.
func (s *Server) handleArticlesPorady(w http.ResponseWriter, r *http.Request) {
	articles, err := s.articleParser.LoadArticlesByCategory(domain.CategoryPorady)
	if err != nil {
		log.Printf("Error loading porady articles: %v", err)
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		return
	}

	// Filter articles based on visibility
	filtered := filterArticlesByVisibility(articles, r)

	data := map[string]interface{}{
		"Title":     "Porady",
		"Articles":  filtered,
		"CartCount": getCartCount(r),
		"CartTotal": getCartTotal(r),
	}

	s.renderTemplate(w, r, "articles.html", data)
}

// handleArticlesRecenzje renders the recenzje (reviews) articles page.
func (s *Server) handleArticlesRecenzje(w http.ResponseWriter, r *http.Request) {
	articles, err := s.articleParser.LoadArticlesByCategory(domain.CategoryRecenzje)
	if err != nil {
		log.Printf("Error loading recenzje articles: %v", err)
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		return
	}

	// Filter articles based on visibility
	filtered := filterArticlesByVisibility(articles, r)

	data := map[string]interface{}{
		"Title":     "Recenzje",
		"Articles":  filtered,
		"CartCount": getCartCount(r),
		"CartTotal": getCartTotal(r),
	}

	s.renderTemplate(w, r, "articles.html", data)
}

// handleArticle renders a single article detail page.
func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	articleID := chi.URLParam(r, "id")

	article, err := s.articleParser.LoadArticleByID(articleID)
	if err != nil {
		log.Printf("Error loading article: %v", err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Check visibility
	if !isItemVisible(article.Visibility, r) {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Load related products if any
	var relatedProducts []domain.Product
	if len(article.RelatedProducts) > 0 {
		for _, productID := range article.RelatedProducts {
			product, err := s.productParser.LoadProductByID(productID)
			if err == nil {
				relatedProducts = append(relatedProducts, *product)
			}
		}
	}

	data := map[string]interface{}{
		"Title":           article.Title,
		"Article":         article,
		"RelatedProducts": relatedProducts,
		"CartCount":       getCartCount(r),
		"CartTotal":       getCartTotal(r),
	}

	s.renderTemplate(w, r, "article.html", data)
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
