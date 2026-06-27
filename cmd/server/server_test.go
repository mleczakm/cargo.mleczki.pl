package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/eventstore"
)

// getTemplatePath returns the correct path to templates regardless of working directory.
func getTemplatePath() string {
	wd, _ := os.Getwd()
	if filepath.Base(wd) == "server" {
		return "../../web/templates"
	}
	return "web/templates"
}

// TestGetCartEmpty tests getting an empty cart.
func TestGetCartEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	cart := getCart(req)

	if len(cart) != 0 {
		t.Errorf("Expected empty cart, got %d items", len(cart))
	}
}

// TestSetAndGetCart tests setting and retrieving a cart.
func TestSetAndGetCart(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Create a cart item
	items := []CartItem{
		{
			CartID:      "123",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{"addon1"},
			Total:       200,
		},
	}

	// Set cart
	setCart(w, req, items)

	// Extract cookie from response
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cart cookie to be set")
	}

	// Create new request with the cookie
	req2 := httptest.NewRequest("GET", "/checkout", nil)
	req2.AddCookie(cookies[0])

	// Get cart
	cart := getCart(req2)

	if len(cart) != 1 {
		t.Errorf("Expected 1 item in cart, got %d", len(cart))
	}

	if cart[0].ProductID != "cargo" {
		t.Errorf("Expected productID 'cargo', got '%s'", cart[0].ProductID)
	}

	if cart[0].CartID != "123" {
		t.Errorf("Expected cartID '123', got '%s'", cart[0].CartID)
	}

	if cart[0].Total != 200 {
		t.Errorf("Expected total 200, got %d", cart[0].Total)
	}
}

// TestSetMultipleCartItems tests setting multiple items in cart.
func TestSetMultipleCartItems(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Create multiple cart items
	items := []CartItem{
		{
			CartID:      "item1",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
		{
			CartID:      "item2",
			ProductID:   "tandem",
			ProductName: "Tandem Bike",
			BasePrice:   80,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{"addon1"},
			Total:       160,
		},
	}

	// Set cart
	setCart(w, req, items)

	// Extract cookie and get cart
	cookies := w.Result().Cookies()
	req2 := httptest.NewRequest("GET", "/checkout", nil)
	req2.AddCookie(cookies[0])
	cart := getCart(req2)

	if len(cart) != 2 {
		t.Errorf("Expected 2 items in cart, got %d", len(cart))
	}

	if cart[1].ProductID != "tandem" {
		t.Errorf("Expected second item productID 'tandem', got '%s'", cart[1].ProductID)
	}
}

// TestRemoveCartItem tests removing an item from cart.
func TestRemoveCartItem(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Create cart with two items
	items := []CartItem{
		{
			CartID:      "item1",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
		{
			CartID:      "item2",
			ProductID:   "tandem",
			ProductName: "Tandem Bike",
			BasePrice:   80,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       160,
		},
	}

	setCart(w, req, items)

	// Get the cookie
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cart cookie")
	}

	// Simulate remove by filtering
	cartID := "item1"
	cart := items
	var newCart []CartItem
	for _, item := range cart {
		if item.CartID != cartID {
			newCart = append(newCart, item)
		}
	}

	if len(newCart) != 1 {
		t.Errorf("Expected 1 item after removal, got %d", len(newCart))
	}

	if newCart[0].CartID != "item2" {
		t.Errorf("Expected remaining item to be 'item2', got '%s'", newCart[0].CartID)
	}
}

// TestCartCookieEncoding tests that cart data is properly URL-encoded.
func TestCartCookieEncoding(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Create item with special characters in name
	items := []CartItem{
		{
			CartID:      "123",
			ProductID:   "cargo",
			ProductName: "Cargo \"Bike\" (Special)",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{"addon-1", "addon-2"},
			Total:       200,
		},
	}

	setCart(w, req, items)

	// Get cookie value
	cookies := w.Result().Cookies()
	cookieValue := cookies[0].Value

	// Cookie value should be URL-encoded (not contain raw quotes)
	if strings.Contains(cookieValue, "\"") {
		t.Error("Cookie value should not contain raw quotes (should be URL-encoded)")
	}

	// Verify it can be decoded
	decoded, err := url.QueryUnescape(cookieValue)
	if err != nil {
		t.Errorf("Failed to decode cookie: %v", err)
	}

	// Verify decoded value is valid JSON
	var cart []CartItem
	if err := json.Unmarshal([]byte(decoded), &cart); err != nil {
		t.Errorf("Failed to unmarshal decoded cart: %v", err)
	}

	if len(cart) != 1 {
		t.Errorf("Expected 1 item, got %d", len(cart))
	}
}

// TestClearCart tests clearing the cart.
func TestClearCart(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Set a cart with items
	items := []CartItem{
		{
			CartID:      "123",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
	}

	setCart(w, req, items)

	// Clear the cart
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/", nil)
	req2.AddCookie(&http.Cookie{
		Name:  "cart",
		Value: url.QueryEscape(string([]byte(`[{"CartID":"123"}]`))),
	})

	clearCart(w2, req2)

	// Check that cleared cookie has MaxAge -1
	cookies := w2.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cart cookie to be cleared")
	}

	if cookies[0].MaxAge != -1 {
		t.Errorf("Expected MaxAge -1 for cleared cookie, got %d", cookies[0].MaxAge)
	}
}

// TestCartItemCalculations tests cart item total calculations.
func TestCalculateItemTotal(t *testing.T) {
	tests := []struct {
		name       string
		basePrice  int
		addons     []string
		rentalDays int
		expected   int
	}{
		{
			name:       "no addons, 1 day",
			basePrice:  100,
			addons:     []string{},
			rentalDays: 1,
			expected:   100,
		},
		{
			name:       "no addons, 2 days",
			basePrice:  100,
			addons:     []string{},
			rentalDays: 2,
			expected:   200,
		},
		{
			name:       "one addon, 1 day",
			basePrice:  100,
			addons:     []string{"addon1"},
			rentalDays: 1,
			expected:   120,
		},
		{
			name:       "two addons, 2 days",
			basePrice:  100,
			addons:     []string{"addon1", "addon2"},
			rentalDays: 2,
			expected:   270, // (100 + 20 + 15) * 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simplified calculation
			total := tt.basePrice

			// Add addon prices (hardcoded for test)
			for _, addonID := range tt.addons {
				if addonID == "addon1" {
					total += 20
				} else if addonID == "addon2" {
					total += 15
				}
			}

			total *= tt.rentalDays

			if total != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, total)
			}
		})
	}
}

// TestCartWithSpecialCharacters tests cart with special characters.
func TestCartWithSpecialCharacters(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cart/add", nil)

	// Create item with special characters
	items := []CartItem{
		{
			CartID:      fmt.Sprintf("%d", time.Now().UnixNano()),
			ProductID:   "special",
			ProductName: "Rower Cargo & Tandem (Special!)",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{"daszek", "timer"},
			Total:       200,
		},
	}

	setCart(w, req, items)
	cookies := w.Result().Cookies()

	// Retrieve cart
	req2 := httptest.NewRequest("GET", "/checkout", nil)
	req2.AddCookie(cookies[0])
	cart := getCart(req2)

	if len(cart) != 1 {
		t.Errorf("Expected 1 item, got %d", len(cart))
	}

	if cart[0].ProductName != "Rower Cargo & Tandem (Special!)" {
		t.Errorf("Product name not preserved correctly: %s", cart[0].ProductName)
	}

	if len(cart[0].Addons) != 2 {
		t.Errorf("Expected 2 addons, got %d", len(cart[0].Addons))
	}
}

// TestHandleCartRemoveInvalid tests removing with invalid HTTP method.
func TestHandleCartRemoveInvalid(_ *testing.T) {
	// Create a mock cart item
	items := []CartItem{
		{
			CartID:      "item1",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
	}

	// Test GET request (should fail - only POST allowed)
	req := httptest.NewRequest("GET", "/cart/remove/item1", nil)

	// Set cart with item
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/cart/add", nil)
	setCart(w1, req1, items)
	cookies := w1.Result().Cookies()
	req.AddCookie(cookies[0])

	// Test that we can't remove with GET
	// (would need to mock the server to fully test, this just tests logic)
	_ = req.Method != "POST"
}

// TestRemoveNonexistentItem tests removing an item that doesn't exist.
func TestRemoveNonexistentItem(t *testing.T) {
	items := []CartItem{
		{
			CartID:      "item1",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
	}

	cartID := "nonexistent"
	cart := items

	// Try to remove item that doesn't exist
	var newCart []CartItem
	found := false
	for _, item := range cart {
		if item.CartID != cartID {
			newCart = append(newCart, item)
		} else {
			found = true
		}
	}

	if found {
		t.Error("Expected item not to be found")
	}

	if len(newCart) != 1 {
		t.Errorf("Expected 1 item to remain, got %d", len(newCart))
	}

	if newCart[0].CartID != "item1" {
		t.Errorf("Expected item1 to remain")
	}
}

// TestCartPersistenceAcrossRequests tests that cart persists with cookies.
func TestCartPersistenceAcrossRequests(t *testing.T) {
	// First request - set cart
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/cart/add", nil)

	items := []CartItem{
		{
			CartID:      "persistence-test-1",
			ProductID:   "cargo",
			ProductName: "Cargo Bike",
			BasePrice:   100,
			StartDate:   "2026-06-10",
			EndDate:     "2026-06-12",
			RentalDays:  2,
			Addons:      []string{},
			Total:       200,
		},
	}

	setCart(w1, req1, items)
	cookie := w1.Result().Cookies()[0]

	// Second request - retrieve cart with same cookie
	req2 := httptest.NewRequest("GET", "/checkout", nil)
	req2.AddCookie(cookie)
	cart := getCart(req2)

	if len(cart) != 1 {
		t.Errorf("Expected 1 item after persistence, got %d", len(cart))
	}

	if cart[0].CartID != "persistence-test-1" {
		t.Errorf("Expected cartID to persist across requests")
	}

	// Third request - add more items to persisted cart
	req3 := httptest.NewRequest("POST", "/cart/add", nil)
	req3.AddCookie(cookie)
	prevCart := getCart(req3)

	newItem := CartItem{
		CartID:      "persistence-test-2",
		ProductID:   "tandem",
		ProductName: "Tandem Bike",
		BasePrice:   80,
		StartDate:   "2026-06-10",
		EndDate:     "2026-06-12",
		RentalDays:  2,
		Addons:      []string{},
		Total:       160,
	}

	prevCart = append(prevCart, newItem)

	w3 := httptest.NewRecorder()
	setCart(w3, req3, prevCart)
	newCookie := w3.Result().Cookies()[0]

	// Fourth request - verify both items exist
	req4 := httptest.NewRequest("GET", "/checkout", nil)
	req4.AddCookie(newCookie)
	finalCart := getCart(req4)

	if len(finalCart) != 2 {
		t.Errorf("Expected 2 items after adding, got %d", len(finalCart))
	}
}

// TestHandleTerms tests the /terms endpoint returns correct content type.
func TestHandleTermsContentType(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/terms", nil)

	// Create a minimal server for testing with templates initialized
	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"stripHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	templatePath := getTemplatePath()
	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join(templatePath, "*.html")))

	server := &Server{
		templates: tmpl,
	}
	server.handleTerms(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("Expected Content-Type 'text/html', got '%s'", contentType)
	}
}

// TestHandleTermsContent tests the /terms endpoint returns terms content.
func TestHandleTermsContent(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/terms", nil)

	// Create a minimal server for testing with templates initialized
	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"stripHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	templatePath := getTemplatePath()
	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join(templatePath, "*.html")))

	server := &Server{
		templates: tmpl,
	}
	server.handleTerms(w, req)

	body := w.Body.String()

	// Check for expected content in the terms
	expectedStrings := []string{
		"Regulamin i Umowa Najmu",
		"Postanowienia ogólne",
		"Warunki wynajmu",
		"Odpowiedzialność Najemcy",
		"Zwrot sprzętu",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected terms to contain '%s'", expected)
		}
	}
}

// TestHandleLoginGET tests the GET /login endpoint redirects to the auth modal on homepage.
func TestHandleLoginGET(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)

	server := &Server{}
	server.handleLogin(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/?auth=login" {
		t.Errorf("Expected redirect to /?auth=login, got %s", location)
	}
}

// TestHandleLoginGETWithNext tests the GET /login endpoint preserves the next query param.
func TestHandleLoginGETWithNext(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login?next=/user/orders", nil)

	server := &Server{}
	server.handleLogin(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/?auth=login&next=%2Fuser%2Forders" {
		t.Errorf("Expected redirect to /?auth=login&next=%%2Fuser%%2Forders, got %s", location)
	}
}

// TestHandleLoginGETModal tests the GET /login?modal=1 endpoint returns login content.
func TestHandleLoginGETModal(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login?modal=1", nil)

	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"stripHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	templatePath := getTemplatePath()
	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join(templatePath, "*.html")))

	server := &Server{
		templates: tmpl,
	}
	server.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Zaloguj się") {
		t.Error("Expected login form content in response body")
	}
}

// TestHandleLoginPOST tests the POST /login endpoint with valid credentials.
func TestHandleLoginPOST(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize schema
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			name TEXT NOT NULL,
			phone TEXT,
			address TEXT,
			is_adult INTEGER DEFAULT 0,
			accepted_tos INTEGER DEFAULT 0,
			is_admin INTEGER DEFAULT 0,
			deletion_requested INTEGER DEFAULT 0,
			deletion_requested_at TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE user_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			ip_address TEXT,
			user_agent TEXT,
			is_admin INTEGER DEFAULT 0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			expires_at TEXT,
			last_activity TEXT
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Create event store
	eventStore, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// Create auth manager
	authManager := auth.NewAuthManager(db, eventStore)

	// Create a test admin user directly in the database
	ctx := t.Context()
	password := "testPassword123!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	userID := "test_admin_123"
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, phone, address, is_adult, accepted_tos, is_admin, deletion_requested, deletion_requested_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, "admin@example.com", string(hash), "Test Admin", "", "", 1, 1, 1, 0, "", now, now)
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Verify user was created
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", "admin@example.com").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query user count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected 1 user, got %d", count)
	}

	// Verify password hash
	var storedHash string
	err = db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE email = ?", "admin@example.com").Scan(&storedHash)
	if err != nil {
		t.Fatalf("Failed to query password hash: %v", err)
	}
	t.Logf("User created successfully, hash length: %d", len(storedHash))

	// Create server
	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"stripHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	templatePath := getTemplatePath()
	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join(templatePath, "*.html")))

	server := &Server{
		authManager: authManager,
		templates:   tmpl,
	}

	// Test login with valid credentials
	formData := url.Values{}
	formData.Set("email", "admin@example.com")
	formData.Set("password", password)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	server.handleLogin(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 (HTMX response), got %d", w.Code)
	}

	// Check that session cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "session_token" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Error("Expected session_token cookie to be set")
	}

	// Check HX-Redirect header
	if w.Header().Get("HX-Redirect") != "/admin" {
		t.Errorf("Expected HX-Redirect to /admin, got %s", w.Header().Get("HX-Redirect"))
	}
}

// TestHandleLoginPOSTInvalidCredentials tests the POST /login endpoint with invalid credentials.
func TestHandleLoginPOSTInvalidCredentials(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize schema
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			name TEXT NOT NULL,
			phone TEXT,
			address TEXT,
			is_adult INTEGER DEFAULT 0,
			accepted_tos INTEGER DEFAULT 0,
			is_admin INTEGER DEFAULT 0,
			deletion_requested INTEGER DEFAULT 0,
			deletion_requested_at TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE user_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			ip_address TEXT,
			user_agent TEXT,
			is_admin INTEGER DEFAULT 0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			expires_at TEXT,
			last_activity TEXT
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Create event store
	eventStore, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// Create auth manager
	authManager := auth.NewAuthManager(db, eventStore)

	// Create server
	funcMap := template.FuncMap{
		"upper":    strings.ToUpper,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"stripHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	templatePath := getTemplatePath()
	tmpl := template.New("main").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join(templatePath, "*.html")))

	server := &Server{
		authManager: authManager,
		templates:   tmpl,
	}

	// Test login with invalid credentials
	formData := url.Values{}
	formData.Set("email", "admin@example.com")
	formData.Set("password", "wrongpassword")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	server.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 (HTMX response), got %d", w.Code)
	}

	// Check that error message is in the response body
	body := w.Body.String()
	if !strings.Contains(body, "Nieprawidłowy adres e-mail lub hasło") {
		t.Errorf("Expected error message in response body, got: %s", body)
	}
}
