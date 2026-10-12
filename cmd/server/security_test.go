package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/transfers"
)

func TestRoutesRejectCrossOriginPostsAndGetLogout(t *testing.T) {
	server := &Server{bankMail: &transfers.BankMailHandler{}}
	router := chi.NewRouter()
	server.RegisterRoutes(router)
	for _, test := range []struct {
		name, method, origin, fetchSite string
		status                          int
	}{
		{"same origin", "POST", "https://cargo.example", "same-origin", http.StatusFound},
		{"foreign origin", "POST", "https://attacker.example", "", http.StatusForbidden},
		{"foreign fetch metadata", "POST", "", "cross-site", http.StatusForbidden},
		{"same site foreign origin", "POST", "https://evil.cargo.example", "same-site", http.StatusForbidden},
		{"non-browser", "POST", "", "", http.StatusFound},
		{"safe method cannot log out", "GET", "", "cross-site", http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), test.method, "https://cargo.example/logout", nil)
			if test.origin != "" {
				req.Header.Set("Origin", test.origin)
			}
			if test.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", test.fetchSite)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if test.status != http.StatusFound && len(response.Result().Cookies()) != 0 {
				t.Fatal("rejected request changed session cookies")
			}
		})
	}
}

func TestRenderedContentKeepsTemplateMarkupButEscapesUserInput(t *testing.T) {
	tmpl := template.Must(template.New("layout.html").Parse(`{{define "layout.html"}}<main>{{.Content}}</main>{{end}}{{define "test-content"}}<strong>{{.Name}}</strong>{{end}}`))
	server := &Server{templates: tmpl}
	response := httptest.NewRecorder()
	server.renderTemplate(response, httptest.NewRequest("GET", "/", nil), "test.html", map[string]interface{}{
		"Name": `<script>alert("xss")</script>`, "ShopNav": false,
	})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "<script>") || !strings.Contains(response.Body.String(), "<strong>&lt;script&gt;") {
		t.Fatalf("unsafe or broken template output: %s", response.Body.String())
	}
}
