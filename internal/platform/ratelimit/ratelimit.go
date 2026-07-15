// Package ratelimit is a real, in-memory fixed-window rate limiter -
// cross-cutting infrastructure, like httpserver/auth/outbox, not owned
// by any one bounded context. Deliberately in-memory, not Redis-backed:
// this codebase runs a single Control Plane instance (the same
// documented limit as the Registry's own Worker directory) - a real,
// named boundary, not silently pretended to be cluster-safe. Multiple
// replicas would each enforce their own independent limit, which is a
// real weakening (an attacker spread across replicas gets N times the
// budget) but never a false negative in the other direction - one
// replica never blocks a request another replica would have allowed
// through its own state.
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	count   int
	resetAt time.Time
}

// Limiter is a fixed-window counter per key - simpler than a sliding
// window or token bucket, and the right tradeoff here: this is defending
// against brute-force/abuse, not smoothing legitimate bursty traffic,
// so the classic "a client could get 2x limit right at a window
// boundary" imprecision fixed-window has doesn't matter for this use
// case the way it would for, say, API billing.
type Limiter struct {
	mu       sync.Mutex
	windows  map[string]*window
	limit    int
	interval time.Duration
}

func New(limit int, interval time.Duration) *Limiter {
	return &Limiter{
		windows:  make(map[string]*window),
		limit:    limit,
		interval: interval,
	}
}

// Allow reports whether key is still within its limit for the current
// window, incrementing its count either way (a rejected request still
// counts - it's still the thing being defended against). retryAfter is
// only meaningful when allowed is false.
func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.windows[key]
	if !ok || now.After(w.resetAt) {
		w = &window{count: 0, resetAt: now.Add(l.interval)}
		l.windows[key] = w
	}

	w.count++
	if w.count > l.limit {
		return false, time.Until(w.resetAt)
	}
	return true, 0
}

// GC evicts expired windows - without this, `windows` grows by one
// entry per distinct key (IP, username, ...) ever seen and never
// shrinks, the same "real, bounded-by-actual-volume, but still
// unbounded" gap this codebase has already named for runToWorker/
// idempotency_keys elsewhere - built with real cleanup from the start
// here instead of deferring it. Called periodically by GCLoop, not on
// every Allow (that would mean every request pays for a full map scan).
func (l *Limiter) GC() {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, w := range l.windows {
		if now.After(w.resetAt) {
			delete(l.windows, key)
		}
	}
}
