package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/middleware"
	"cargo.mleczki.pl/internal/projections"
)

func TestSessionMiddlewareAndGetUserID(t *testing.T) {
	readModels, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer readModels.Close()
	db := readModels.GetDB()
	_, err = db.Exec("INSERT INTO users (id, email, name, password_hash, phone, address, is_admin) VALUES ('u1', 'u@example.com', 'User', '', '', '', 1)")
	if err != nil {
		t.Fatal(err)
	}
	for token, expiry := range map[string]time.Time{"valid": time.Now().Add(time.Hour), "expired": time.Now().Add(-time.Hour)} {
		if _, err := db.Exec("INSERT INTO user_sessions (id, user_id, is_admin, expires_at) VALUES (?, 'u1', 1, ?)", token, expiry.UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	manager := auth.NewAuthManager(db, nil)
	for _, token := range []string{"", "missing", "expired", "valid"} {
		t.Run(token, func(t *testing.T) {
			called := false
			handler := middleware.SessionMiddleware(manager)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				wantID := ""
				if token == "valid" {
					wantID = "u1"
				}
				if got := middleware.GetUserID(r); got != wantID {
					t.Errorf("GetUserID = %q, want %q", got, wantID)
				}
				if middleware.IsAuthenticated(r) != (token == "valid") || middleware.IsAdmin(r) != (token == "valid") {
					t.Error("incorrect authentication context")
				}
			}))
			req := httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
			if token != "" {
				req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Error("middleware did not call the next handler")
			}
		})
	}
}
