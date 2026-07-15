package httpserver

import (
	"context"
	"net/http"
	"strings"

	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/principal"
)

type contextKey int

const userIDContextKey contextKey = iota

// APIKeyResolver resolves a presented opaque API key (plaintext, as the
// caller sent it) to the subject id it authenticates as - a
// ServiceAccount's id, in this codebase's only real caller today
// (cmd/control-plane/main.go wires this over identity's
// APIKeyRepository.GetByHash + auth.HashOpaqueToken) - plus that key's
// own Scopes (possibly empty, meaning "no narrowing" - see
// principal.WithScopes's own comment). Declared here, not imported from
// Identity, per this codebase's own no-cross-context-import rule -
// httpserver is cross-cutting infrastructure, it doesn't get to depend
// on any one bounded context's domain package.
type APIKeyResolver func(ctx context.Context, plaintextKey string) (subjectID string, scopes []string, err error)

// RequireAuth parses `Authorization: Bearer <token>`, verifies it, and
// puts the authenticated subject's id on the request context - every
// org-scoped handler downstream reads the Principal's identity from
// here, never from a client-supplied field, per
// docs/architecture/04-api-design.md §4's Principal model. Accepts
// EITHER a JWT access token OR a real API key
// (docs/architecture/13-module-identity-rbac-tenancy.md §2) - resolved
// via resolveAPIKey, nil-safe (a nil resolver just means "this deployment
// doesn't wire API-key auth," not a panic). The two are told apart
// structurally, not by trying one then falling back on any failure: a
// JWT is always three dot-separated segments (a fixed property of the
// format itself), this codebase's own opaque tokens
// (internal/platform/auth.GenerateOpaqueToken, base64.RawURLEncoding)
// never contain a '.' - so an invalid JWT is still reported as an
// invalid JWT, not silently retried as a malformed API key.
func RequireAuth(secret []byte, resolveAPIKey APIKeyResolver, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			WriteProblem(w, http.StatusUnauthorized, "missing or malformed Authorization header", "")
			return
		}

		var subjectID string
		var scopes []string
		if strings.Count(token, ".") == 2 {
			userID, err := auth.ParseAccessToken(secret, token)
			if err != nil {
				WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
				return
			}
			subjectID = userID
		} else {
			if resolveAPIKey == nil {
				WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
				return
			}
			resolved, resolvedScopes, err := resolveAPIKey(r.Context(), token)
			if err != nil {
				WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
				return
			}
			subjectID = resolved
			scopes = resolvedScopes
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, subjectID)
		// A JWT-authenticated request never carries a scope restriction
		// (scopes is nil here) - principal.WithScopes(ctx, nil) is a
		// real, deliberate no-op, matching ScopesFromContext's own
		// "empty means unrestricted" contract.
		ctx = principal.WithScopes(ctx, scopes)
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
