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
