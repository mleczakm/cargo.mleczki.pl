package shop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/shop"
)

func writeItem(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestParserDirectItemWithStockAndVariants(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "kask.md", `---
id: kask
name: Kask rowerowy
category: akcesoria
price: 120
image: kask.jpg
stock:
  lowThreshold: 2
  backorder: true
  leadTime: 5-7 dni
  maxPerOrder: 4
variants:
  - sku: kask-s
    name: Rozmiar S
    stock: 3
  - sku: kask-l
    name: Rozmiar L
    price: 130.50
    stock: 1
relatedProducts: [cargo]
relatedArticles: [trekking-bike-guide]
priority: 5
---
Lekki kask. Pasuje do [[product:cargo]].
`)

	items, err := shop.NewParser(dir).LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	item := items[0]

	if !item.IsDirect() || !item.Tracked() {
		t.Errorf("item should be a tracked direct item: %+v", item)
	}
	if item.Image != "/data/images/shop/kask.jpg" {
		t.Errorf("image not normalised: %q", item.Image)
	}
	if got := item.SKUs(); len(got) != 2 || got[0] != "kask-s" || got[1] != "kask-l" {
		t.Errorf("SKUs = %v", got)
	}
	if item.UnitPrice("kask-s") != domain.Zloty(120) || item.UnitPrice("kask-l") != domain.PLN(13050) {
		t.Errorf("variant prices wrong: %s / %s", item.UnitPrice("kask-s"), item.UnitPrice("kask-l"))
	}
	if item.InitialStockFor("kask-s") != 3 || item.InitialStockFor("kask-l") != 1 {
		t.Error("variant initial stock not read")
	}
	if item.LineName("kask-l") != "Kask rowerowy – Rozmiar L" {
		t.Errorf("LineName = %q", item.LineName("kask-l"))
	}
	if !item.Stock.Backorder || item.Stock.LeadTime != "5-7 dni" || item.MaxQuantity() != 4 || item.LowThreshold() != 2 {
		t.Errorf("stock policy not read: %+v", item.Stock)
	}
	if !strings.Contains(item.Description, "[[product:cargo]]") {
		t.Errorf("shortcode must survive markdown rendering, got %q", item.Description)
	}
	if strings.Contains(item.Summary, "[[") || !strings.HasPrefix(item.Summary, "Lekki kask.") {
		t.Errorf("summary should be plain text without shortcodes, got %q", item.Summary)
	}
}

func TestParserAcceptsPricesWithGrosze(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "a.md", "---\nid: a\nname: A\nprice: 49.99\n---\nx")
	writeItem(t, dir, "b.md", "---\nid: b\nname: B\nprice: '12,5'\n---\nx")
	writeItem(t, dir, "c.md", "---\nid: c\nname: C\nprice: 150\n---\nx")

	items, err := shop.NewParser(dir).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.Money{}
	for _, item := range items {
		got[item.ID] = item.Price
	}
	if got["a"] != domain.PLN(4999) || got["b"] != domain.PLN(1250) || got["c"] != domain.Zloty(150) {
		t.Errorf("prices = %v", got)
	}
}

func TestParserAffiliateItem(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "lampka.md", `---
id: lampka
name: Lampka
affiliate:
  url: https://allegro.pl/oferta/lampka-123
  priceHint: od ok. 59 zł
---
Dobra lampka.
`)
	items, err := shop.NewParser(dir).LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	item := items[0]
	if !item.IsAffiliate() || item.Tracked() {
		t.Fatalf("expected an untracked affiliate item: %+v", item)
	}
	if item.Affiliate.Merchant != "Allegro" || item.Affiliate.PriceHint != "od ok. 59 zł" {
		t.Errorf("affiliate not read: %+v", item.Affiliate)
	}
	if item.Affiliate.Label != "Zobacz ofertę w serwisie Allegro" {
		t.Errorf("default label = %q", item.Affiliate.Label)
	}
}

func TestParserRejectsInvalidItemsButKeepsTheRest(t *testing.T) {
	cases := map[string]string{
		"too-precise.md":        "---\nid: a\nname: A\nprice: 49.999\n---\nx",
		"no-price.md":           "---\nid: b\nname: B\n---\nx",
		"bad-id.md":             "---\nid: Bad_Id\nname: C\nprice: 5\n---\nx",
		"affiliate-bad.md":      "---\nid: d\nname: D\naffiliate:\n  url: javascript:alert(1)\n---\nx",
		"affiliate-price.md":    "---\nid: e\nname: E\nprice: 5\naffiliate:\n  url: https://allegro.pl/x\n---\nx",
		"both.md":               "---\nid: f\nname: F\ntype: direct\nprice: 5\naffiliate:\n  url: https://allegro.pl/x\n---\nx",
		"visibility.md":         "---\nid: g\nname: G\nprice: 5\nvisibility: secret\n---\nx",
		"no-frontmatter.md":     "just text",
		"dup-variant.md":        "---\nid: h\nname: H\nprice: 5\nvariants:\n  - {sku: v, name: V}\n  - {sku: v, name: W}\n---\nx",
		"stock-and-variants.md": "---\nid: i\nname: I\nprice: 5\nstock: {initial: 4}\nvariants:\n  - {sku: v1, name: V}\n---\nx",
	}
	dir := t.TempDir()
	for name, content := range cases {
		writeItem(t, dir, name, content)
	}
	writeItem(t, dir, "good.md", "---\nid: good\nname: Good\nprice: 10\n---\nok")

	items, err := shop.NewParser(dir).LoadAll()
	if len(items) != 1 || items[0].ID != "good" {
		t.Fatalf("only the valid item should load, got %+v", items)
	}
	if err == nil {
		t.Fatal("expected problems to be reported")
	}
	for name := range cases {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("problem for %s not reported in: %v", name, err)
		}
	}
}

func TestParserRejectsDuplicateIDsAndSKUs(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "a.md", "---\nid: same\nname: First\nprice: 5\n---\nx")
	writeItem(t, dir, "b.md", "---\nid: same\nname: Second\nprice: 5\n---\nx")
	writeItem(t, dir, "c.md", "---\nid: c\nname: C\nprice: 5\nvariants:\n  - {sku: same, name: V}\n---\nx")

	items, err := shop.NewParser(dir).LoadAll()
	if len(items) != 1 || items[0].Name != "First" {
		t.Fatalf("the first definition must win, got %+v", items)
	}
	if err == nil || !strings.Contains(err.Error(), "b.md") || !strings.Contains(err.Error(), "c.md") {
		t.Errorf("duplicates must be reported, got %v", err)
	}
}

func TestParserOrdersByPriorityThenNameAndFindsByID(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "1.md", "---\nid: b-item\nname: B\nprice: 1\npriority: 1\n---\nx")
	writeItem(t, dir, "2.md", "---\nid: a-item\nname: A\nprice: 1\npriority: 1\n---\nx")
	writeItem(t, dir, "3.md", "---\nid: top\nname: Z\nprice: 1\npriority: 9\n---\nx")

	catalog := shop.NewParser(dir)
	items, _ := catalog.LoadAll()
	if got := []string{items[0].ID, items[1].ID, items[2].ID}; got[0] != "top" || got[1] != "a-item" || got[2] != "b-item" {
		t.Errorf("order = %v", got)
	}
	if item, err := catalog.LoadByID("b-item"); err != nil || item.Name != "B" {
		t.Errorf("LoadByID: %v %v", item, err)
	}
	if _, err := catalog.LoadByID("nope"); err == nil {
		t.Error("expected an error for an unknown id")
	}
}

func TestParserMissingDirectoryIsAnEmptyShop(t *testing.T) {
	items, err := shop.NewParser(filepath.Join(t.TempDir(), "absent")).LoadAll()
	if err != nil || len(items) != 0 {
		t.Fatalf("missing directory should be an empty shop, got %v %v", items, err)
	}
}

func TestMerchantName(t *testing.T) {
	cases := map[string]string{
		"allegro.pl":         "Allegro",
		"pl.aliexpress.com":  "AliExpress",
		"www.amazon.de":      "Amazon",
		"www.example.com":    "example.com",
		"Allegro Lokalnie":   "Allegro",
		"Sklep Rowerowy.pl ": "Sklep Rowerowy.pl",
	}
	for in, want := range cases {
		if got := shop.MerchantName(in); got != want {
			t.Errorf("MerchantName(%q) = %q, want %q", in, got, want)
		}
	}
}
