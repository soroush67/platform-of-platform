package httpserver

import (
	"context"
	"net/http"
	"strings"

	"platform-of-platform/internal/platform/auth"
)

type contextKey int

const userIDContextKey contextKey = iota

// RequireAuth parses `Authorization: Bearer <token>`, verifies it, and
// puts the authenticated user's id on the request context - every
// org-scoped handler downstream reads the Principal's identity from
// here, never from a client-supplied field, per
// docs/architecture/04-api-design.md §4's Principal model.
func RequireAuth(secret []byte, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			WriteProblem(w, http.StatusUnauthorized, "missing or malformed Authorization header", "")
			return
		}

		userID, err := auth.ParseAccessToken(secret, token)
		if err != nil {
			WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

// UserIDFromContext returns the authenticated user's id set by
// RequireAuth - ok is false if called on a request that never went
// through RequireAuth (a handler-wiring bug, not a runtime condition to
// silently tolerate).
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}
