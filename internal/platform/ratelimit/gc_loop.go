package ratelimit

import (
	"context"
	"time"
)

// GCLoop implements the Runnable interface
// (docs/architecture/18-backend-structure.md §4), same shape as every
// other background sweep in this codebase (outbox.Relay,
// execution.StaleRunReaperService, tenancy.PurgeReaperService) -
// registered directly in main.go's errgroup. Takes every Limiter this
// deployment runs (login + general, today) so main.go only needs one
// ticker goroutine, not one per limiter.
type GCLoop struct {
	limiters []*Limiter
	interval time.Duration
}

func NewGCLoop(interval time.Duration, limiters ...*Limiter) *GCLoop {
	return &GCLoop{limiters: limiters, interval: interval}
}

func (g *GCLoop) Run(ctx context.Context) error {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, l := range g.limiters {
				l.GC()
			}
		}
	}
}
