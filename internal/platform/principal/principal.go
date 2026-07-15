// Package principal carries one narrow, cross-cutting fact through a
// request's context.Context: an optional permission-scope restriction
// on the currently-authenticated subject. Its only real producer is
// httpserver.RequireAuth (when a request authenticates via an API key
// that has its own Scopes - internal/identity/domain/api_key.go's
// "optional narrowing below the owner's own RBAC grants"); its only real
// consumer is RoleBindingRepository.HasPermission/HasPermissionAtScope
// (internal/rbac/adapters/postgres) - real intersection at the one
// place every permission check in this codebase already funnels
// through, deliberately instead of threading an extra parameter through
// every application service's Execute() call across all seven bounded
// contexts, a materially larger, previously-deferred change.
//
// Lives under internal/platform (like httpserver/auth/outbox), not
// inside RBAC or Identity - httpserver setting a value RBAC's postgres
// adapter reads would otherwise mean either RBAC importing httpserver
// (an adapter depending on a different context's adapter layer) or
// httpserver importing RBAC's domain (the exact cross-context coupling
// this codebase's own dependency-inversion rule forbids). A small,
// genuinely neutral package both sides can depend on is the real fix.
package principal

import "context"

type contextKey int

const scopesKey contextKey = iota

// WithScopes attaches a permission-scope restriction to ctx. An empty
// or nil slice means "no restriction" (the request authenticated via a
// JWT, or via an API key whose own Scopes list is empty) - the same
// value either way, callers don't need to distinguish "never called"
// from "called with an empty slice."
func WithScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, scopesKey, scopes)
}

// ScopesFromContext returns the restriction WithScopes attached, and
// whether the restriction is real (non-empty) - HasPermissionAtScope
// only actually intersects when ok is true; an absent or empty
// restriction means "evaluate RBAC alone, same as before this existed."
func ScopesFromContext(ctx context.Context) (scopes []string, ok bool) {
	scopes, _ = ctx.Value(scopesKey).([]string)
	return scopes, len(scopes) > 0
}
