package tmdbtv

import (
	"sync"
	"time"
)

// responseCache is a small TTL cache for TMDB GET responses, keyed by the
// request URL (path + query, excluding the API key). TMDB metadata changes
// slowly and popular/trending/genre responses are identical across every
// user, so collapsing duplicate calls for a few minutes cuts both latency and
// the risk of hitting TMDB's rate limit. Safe for concurrent use.
type responseCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
	max     int
	now     func() time.Time // injectable for tests
}

type cacheEntry struct {
	body      []byte
	expiresAt time.Time
}

func newResponseCache(ttl time.Duration, max int) *responseCache {
	return &responseCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		max:     max,
		now:     time.Now,
	}
}

// get returns the cached body for key if present and unexpired.
func (c *responseCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.now().After(e.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.body, true
}

// set stores body under key with the cache TTL. When the cache exceeds its
// size cap it first drops expired entries, then evicts arbitrary entries until
// back under the cap — bounding memory without the overhead of full LRU.
func (c *responseCache) set(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		now := c.now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		for k := range c.entries {
			if len(c.entries) < c.max {
				break
			}
			delete(c.entries, k)
		}
	}
	c.entries[key] = cacheEntry{
		body:      body,
		expiresAt: c.now().Add(c.ttl),
	}
}
