package products_test

import (
	"os"
	"path/filepath"
	"testing"

	"cargo.mleczki.pl/internal/products"
)

func TestParser_LoadAllProducts(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test product files
	testProducts := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "test1.md",
			content: `---
id: test1
name: Test Product 1
basePrice: 100
image: https://example.com/image1.jpg
icon: 🚲
bookedDates: []
addons: []
---

Test description 1
`,
			expected: "test1",
		},
		{
			name: "test2.md",
			content: `---
id: test2
name: Test Product 2
basePrice: 200
image: https://example.com/image2.jpg
icon: 🚗
bookedDates: []
addons: []
---

Test description 2
`,
			expected: "test2",
		},
	}

	for _, tp := range testProducts {
		err := os.WriteFile(filepath.Join(tmpDir, tp.name), []byte(tp.content), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create parser with test directory
	parser := products.NewParser(tmpDir)

	// Load all products
	productList, err := parser.LoadAllProducts()
	if err != nil {
		t.Fatalf("LoadAllProducts failed: %v", err)
	}

	// Verify we got 2 products
	if len(productList) != 2 {
		t.Errorf("Expected 2 products, got %d", len(productList))
	}

	// Verify product IDs
	productIDs := make(map[string]bool)
	for _, p := range productList {
		productIDs[p.ID] = true
	}

	for _, tp := range testProducts {
		if !productIDs[tp.expected] {
			t.Errorf("Expected product ID %s not found", tp.expected)
		}
	}
}

func TestParser_LoadProductByID(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test product file
	content := `---
id: test-product
name: Test Product
basePrice: 150
image: https://example.com/image.jpg
icon: 🚲
bookedDates:
  - "2026-06-10"
  - "2026-06-11"
addons:
  - id: addon1
    name: Test Addon
    price: 20
    icon: ⭐
---

Test product description with **markdown** formatting.
`

	err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create parser with test directory
	parser := products.NewParser(tmpDir)

	// Load product by ID
	product, err := parser.LoadProductByID("test-product")
	if err != nil {
		t.Fatalf("LoadProductByID failed: %v", err)
	}

	// Verify product details
	if product.ID != "test-product" {
		t.Errorf("Expected ID test-product, got %s", product.ID)
	}

	if product.Name != "Test Product" {
		t.Errorf("Expected name 'Test Product', got %s", product.Name)
	}

	if product.BasePrice != 150 {
		t.Errorf("Expected base price 150, got %d", product.BasePrice)
	}

	if len(product.BookedDates) != 2 {
		t.Errorf("Expected 2 booked dates, got %d", len(product.BookedDates))
	}

	if len(product.Addons) != 1 {
		t.Errorf("Expected 1 addon, got %d", len(product.Addons))
	}

	if product.Addons[0].Name != "Test Addon" {
		t.Errorf("Expected addon name 'Test Addon', got %s", product.Addons[0].Name)
	}

	// Verify default payment type is "daily" when not specified
	if product.Addons[0].PaymentType != "daily" {
		t.Errorf("Expected default payment type 'daily', got %s", product.Addons[0].PaymentType)
	}
}

func TestParser_LoadProductByID_NotFound(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create parser with empty directory
	parser := products.NewParser(tmpDir)

	// Try to load non-existent product
	_, err := parser.LoadProductByID("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent product, got nil")
	}
}

func TestParser_AddonPaymentType(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test product file with mixed payment types
	content := `---
id: test-payment-types
name: Test Product
basePrice: 100
image: https://example.com/image.jpg
icon: 🚲
bookedDates: []
addons:
  - id: daily-addon
    name: Daily Addon
    price: 10
    icon: 📅
    paymentType: daily
  - id: one-time-addon
    name: One-Time Addon
    price: 50
    icon: 💰
    paymentType: one-time
  - id: default-addon
    name: Default Addon
    price: 15
    icon: ⭐
---

Test product description
`

	err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create parser with test directory
	parser := products.NewParser(tmpDir)

	// Load product
	product, err := parser.LoadProductByID("test-payment-types")
	if err != nil {
		t.Fatalf("LoadProductByID failed: %v", err)
	}

	// Verify we have 3 addons
	if len(product.Addons) != 3 {
		t.Fatalf("Expected 3 addons, got %d", len(product.Addons))
	}

	// Verify daily addon
	if product.Addons[0].PaymentType != "daily" {
		t.Errorf("Expected daily addon to have paymentType 'daily', got %s", product.Addons[0].PaymentType)
	}

	// Verify one-time addon
	if product.Addons[1].PaymentType != "one-time" {
		t.Errorf("Expected one-time addon to have paymentType 'one-time', got %s", product.Addons[1].PaymentType)
	}

	// Verify default addon (should default to daily)
	if product.Addons[2].PaymentType != "daily" {
		t.Errorf("Expected default addon to have paymentType 'daily', got %s", product.Addons[2].PaymentType)
	}
}
