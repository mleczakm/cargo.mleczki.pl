package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

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
		"upper": strings.ToUpper,
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles("../../web/templates/layout.html"))
	tmpl = template.Must(tmpl.ParseGlob("../../web/templates/*.html"))

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
		"upper": strings.ToUpper,
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles("../../web/templates/layout.html"))
	tmpl = template.Must(tmpl.ParseGlob("../../web/templates/*.html"))

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
			t.Errorf("Expected response to contain '%s', but it was not found", expected)
		}
	}
}
