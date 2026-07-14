package idempotency

import (
	"bytes"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
)

// Middleware implements docs/architecture/04-api-design.md §5.
// Deliberately optional, not required - the doc's own wording is
// endpoints "accept" an Idempotency-Key header, not that they reject a
// request missing one: a caller who doesn't supply a key just gets the
// existing, undeduplicated behavior, same as every endpoint before this
// existed. Must run *after* httpserver.RequireAuth in the handler chain
// (it reads the authenticated user id from the request context) and
// only makes sense on routes with an `{id}` (organization) path
// parameter - the same convention every route in this codebase already
// uses for the org id.
func Middleware(store *Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next(w, r)
			return
		}

		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		organizationID := r.PathValue("id")

		cached, err := store.Get(r.Context(), organizationID, userID, key)
		if err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "idempotency check failed", "")
			return
		}
		if cached != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(cached.Status)
			w.Write(cached.Body)
			return
		}

		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)

		// Only successful responses get cached - if the first attempt
		// never actually took effect (400/403/404/409/500), there's no
		// duplicate-creation risk in simply letting a retry attempt the
		// operation again for real, and caching a transient 500 for a
		// full 24h would actively punish a legitimate retry after a
		// blip rather than protect anything.
		if rec.status >= 200 && rec.status < 300 {
			_ = store.Save(r.Context(), organizationID, userID, key, rec.status, rec.body.Bytes())
		}
	}
}

// recorder captures what the real handler wrote while still passing it
// straight through to the real client - the request that actually
// triggered the operation gets its normal response either way; the
// capture only feeds Store.Save for *future* replays.
type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
