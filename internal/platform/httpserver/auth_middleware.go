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
		subjectID, scopes, ok, err := authenticate(r, secret, resolveAPIKey)
		if err != nil || !ok {
			WriteProblem(w, http.StatusUnauthorized, "missing or malformed Authorization header", "")
			return
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

// authenticate is RequireAuth's own token-parsing logic, extracted so
// RequireAuthOrFirstUserBootstrap below can reuse it exactly rather than
// drifting out of sync with a second, hand-copied implementation. ok is
// false only for a genuinely absent Authorization header (empty token) -
// a *present but invalid* one still reports it via err/ok=false too, but
// the caller distinguishes "absent" itself by checking the header first
// where that distinction matters (see RequireAuthOrFirstUserBootstrap).
func authenticate(r *http.Request, secret []byte, resolveAPIKey APIKeyResolver) (subjectID string, scopes []string, ok bool, err error) {
	header := r.Header.Get("Authorization")
	token, hasBearer := strings.CutPrefix(header, "Bearer ")
	if !hasBearer || token == "" {
		return "", nil, false, nil
	}

	if strings.Count(token, ".") == 2 {
		userID, parseErr := auth.ParseAccessToken(secret, token)
		if parseErr != nil {
			return "", nil, false, parseErr
		}
		return userID, nil, true, nil
	}

	if resolveAPIKey == nil {
		return "", nil, false, nil
	}
	resolved, resolvedScopes, resolveErr := resolveAPIKey(r.Context(), token)
	if resolveErr != nil {
		return "", nil, false, resolveErr
	}
	return resolved, resolvedScopes, true, nil
}

// UserCountChecker is httpserver's own tiny port into Identity's
// UserRepository.Count - a plain func type, not an imported interface,
// same "cross-cutting infrastructure doesn't depend on any one bounded
// context's domain package" posture APIKeyResolver above already
// documents.
type UserCountChecker func(ctx context.Context) (int, error)

// RequireAuthOrFirstUserBootstrap is POST /api/v1/users's own route
// wiring (cmd/control-plane/main.go) - real users normally need
// RequireAuth like every other mutating route, but a genuinely fresh
// deployment with zero rows in the users table has no token to present
// yet and no other way to create its first login-capable account at
// all. This is the bootstrap escape hatch: a request presenting NO
// Authorization header at all (not an invalid one - that still hard-
// fails, same as RequireAuth) is let through unauthenticated only when
// countUsers reports zero. Once any user exists, every future call
// (bootstrap or not) goes through the normal RequireAuth path - this
// window closes itself after the very first successful call.
//
// Known, accepted narrow race: two concurrent unauthenticated requests
// arriving before the first user row commits could both see count==0
// and both succeed, creating two "first" users instead of one. Not
// worth guarding against - this is a one-time, operator-controlled boot
// sequence, not a standing production attack surface, and no other
// bootstrap-style operation in this codebase (e.g. Vault's own AppRole
// init) is hardened against this class of race either.
func RequireAuthOrFirstUserBootstrap(secret []byte, resolveAPIKey APIKeyResolver, countUsers UserCountChecker, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if _, hasBearer := strings.CutPrefix(header, "Bearer "); hasBearer && header != "Bearer " {
			// A token WAS presented - authenticate it for real, same as
			// RequireAuth, no bootstrap carve-out once someone's trying
			// to actually log in.
			subjectID, scopes, ok, err := authenticate(r, secret, resolveAPIKey)
			if err != nil || !ok {
				WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
				return
			}
			ctx := context.WithValue(r.Context(), userIDContextKey, subjectID)
			ctx = principal.WithScopes(ctx, scopes)
			next(w, r.WithContext(ctx))
			return
		}

		count, err := countUsers(r.Context())
		if err != nil {
			WriteProblem(w, http.StatusInternalServerError, "failed to check bootstrap eligibility", "")
			return
		}
		if count > 0 {
			WriteProblem(w, http.StatusUnauthorized, "missing or malformed Authorization header", "")
			return
		}

		// No Authorization header AND zero existing users - the real
		// bootstrap case. No subject id in context (CreateUserHandler
		// never reads one - user creation is org-independent by design,
		// see CreateUserService's own doc comment).
		next(w, r)
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
