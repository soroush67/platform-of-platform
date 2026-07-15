package httpserver

import (
	"net"
	"net/http"
	"strconv"

	"platform-of-platform/internal/platform/ratelimit"
)

// RateLimit wraps the whole mux (applied once in main.go, same pattern
// as RequestID) - a general, per-client-IP defense against abuse,
// independent of the stricter, per-username limiter LoginHandler applies
// on top of this for the one endpoint (login) where the more valuable
// key is "which account is being attacked," not "which IP." Every
// request pays this check, authenticated or not - the same posture
// RequestID already established (it has to run before RequireAuth even
// knows who's asking).
func RateLimit(limiter *ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		allowed, retryAfter := limiter.Allow(ip)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			WriteProblem(w, http.StatusTooManyRequests, "too many requests", "")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP strips the port from RemoteAddr - this deployment has no
// reverse proxy in front of it yet (docker-compose.yml publishes the
// Control Plane's own port directly), so RemoteAddr is the real client
// address, not something to trust an X-Forwarded-For header for; a real
// deployment behind a load balancer would need to source this from a
// trusted forwarding header instead, a further, real, not-yet-needed
// gap in this dev/single-instance topology.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
