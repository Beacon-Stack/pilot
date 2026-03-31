// Package ratelimit provides per-key token-bucket rate limiters for indexer queries.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	limiter   *rate.Limiter
	reqPerMin int
}

// Registry manages per-indexer rate limiters keyed by indexer config ID.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Wait blocks until the rate limiter for the given ID allows a request.
// If reqPerMin is 0 (unlimited), it returns immediately.
// The limiter is created on first use and updated if reqPerMin changes.
func (r *Registry) Wait(ctx context.Context, id string, reqPerMin int) error {
	if reqPerMin <= 0 {
		return nil
	}

	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok || e.reqPerMin != reqPerMin {
		e = &entry{
			limiter:   rate.NewLimiter(rate.Every(time.Minute/time.Duration(reqPerMin)), 1),
			reqPerMin: reqPerMin,
		}
		r.entries[id] = e
	}
	r.mu.Unlock()

	return e.limiter.Wait(ctx)
}

// Remove deletes the limiter for the given ID, freeing memory.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	delete(r.entries, id)
	r.mu.Unlock()
}
