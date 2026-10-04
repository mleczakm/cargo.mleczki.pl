package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func renderAdminPanel(t *testing.T, address string) string {
	t.Helper()
	funcs := template.FuncMap{
		"upper":     strings.ToUpper,
		"safeHTML":  func(s string) template.HTML { return template.HTML(s) }, // #nosec G203 // test input
		"stripHTML": func(s string) template.HTML { return template.HTML(s) }, // #nosec G203 // test input
	}
	tmpl, err := template.New("main").Funcs(funcs).ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	data := map[string]interface{}{"BankMailAddress": address, "Transfers": []map[string]interface{}{}}
	if err := tmpl.ExecuteTemplate(&out, "admin_panel-content", data); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestAdminPanelShowsBankMailAddressWithCopyButton(t *testing.T) {
	html := renderAdminPanel(t, "abc123@cargo.mleczki.pl")
	for _, want := range []string{`value="abc123@cargo.mleczki.pl"`, `copyBankMailAddress(this)`, "Kopiuj"} {
		if !strings.Contains(html, want) {
			t.Errorf("admin panel missing %q", want)
		}
	}
}

func TestAdminPanelExplainsMissingBankMailAddress(t *testing.T) {
	html := renderAdminPanel(t, "")
	if strings.Contains(html, "copyBankMailAddress(this)") || !strings.Contains(html, "Adres nie jest jeszcze skonfigurowany") {
		t.Error("expected a not-configured notice without a copy button")
	}
}
