package httpserver

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// contextKey itself is declared in auth_middleware.go - shared within
// this package, one iota sequence, so userIDContextKey and
// requestIDContextKey can never collide.
const requestIDContextKey contextKey = iota + 1

// RequestID wraps the whole mux (applied once in main.go, not per-route
// like RequireAuth) so every request - authenticated or not, including
// /healthz and the pre-auth /users and /auth/login endpoints - gets one.
// Honors an incoming X-Request-ID from the caller (a real deployment
// sitting behind a gateway/load balancer that already generates one per
// request should have its id survive end to end, not get silently
// replaced), generating a real UUID only when the caller didn't supply
// one. Set on the response header immediately, before the handler runs,
// specifically so WriteProblem (internal/platform/httpserver/problem.go)
// can read it straight back off the ResponseWriter's own headers - no
// signature change needed on WriteProblem or any of its ~25 existing
// call sites across every context's HTTP adapter.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey, id)))
	})
}

// RequestIDFromContext lets a handler/service log with the same
// correlation id a client would see in the response header or a 500
// body, without needing to thread it through as an explicit parameter.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}
