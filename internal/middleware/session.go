package middleware

import (
	"context"
	"net/http"

	"cargo.mleczki.pl/internal/auth"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	contextKeyUser    contextKey = "user"
	contextKeyIsAdmin contextKey = "is_admin"
	contextKeyUserID  contextKey = "user_id"
)

// SessionMiddleware adds user information to the request context from session cookie.
func SessionMiddleware(authManager *auth.AuthManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for session cookie
			cookie, err := r.Cookie("session_token")
			if err != nil {
				// No session cookie, continue without user context
				next.ServeHTTP(w, r)
				return
			}

			// Verify session
			user, isAdmin, err := authManager.VerifySession(r.Context(), cookie.Value)
			if err != nil {
				// Invalid session, continue without user context
				next.ServeHTTP(w, r)
				return
			}

			// Add user to request context
			ctx := context.WithValue(r.Context(), contextKeyUser, user)
			ctx = context.WithValue(ctx, contextKeyIsAdmin, isAdmin)
			ctx = context.WithValue(ctx, contextKeyUserID, user.ID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID retrieves the user ID from the request context.
func GetUserID(r *http.Request) string {
	if userID, ok := r.Context().Value(contextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

// GetUser retrieves the user from the request context.
func GetUser(r *http.Request) interface{} {
	return r.Context().Value(contextKeyUser)
}

// IsAdmin checks if the current user is an admin.
func IsAdmin(r *http.Request) bool {
	if isAdmin, ok := r.Context().Value(contextKeyIsAdmin).(bool); ok {
		return isAdmin
	}
	return false
}

// IsAuthenticated checks if a user is authenticated.
func IsAuthenticated(r *http.Request) bool {
	return GetUser(r) != nil
}
