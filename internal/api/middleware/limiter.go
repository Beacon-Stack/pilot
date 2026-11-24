package middleware

import (
	"sync"
	"time"
)

// attemptLimiter slows password guessing. After freeAttempts wrong
// passwords in a row from one address, that address has to wait before
// the next try: one minute, doubling with every further miss up to
// maxLockout. A correct password clears it. The password hash already
// makes each guess slow; this makes a long run of them pointless.
//
// It is keyed on the address that connected, never on forwarded headers:
// those are the caller's to choose, and a guesser could change them on
// every try.
type attemptLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	byKey map[string]*attempts
}

type attempts struct {
	fails int
	until time.Time // no tries before this
	last  time.Time
}

const (
	freeAttempts  = 5
	firstLockout  = time.Minute
	maxLockout    = 15 * time.Minute
	forgetAfter   = time.Hour // misses older than this no longer count
	maxLimiterKey = 10_000
)

func newAttemptLimiter() *attemptLimiter {
	return &attemptLimiter{now: time.Now, byKey: make(map[string]*attempts)}
}

// wait reports how long key must still wait before trying again; zero
// means go ahead.
func (l *attemptLimiter) wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.byKey[key]
	if a == nil {
		return 0
	}
	now := l.now()
	if now.Sub(a.last) > forgetAfter {
		delete(l.byKey, key)
		return 0
	}
	if d := a.until.Sub(now); d > 0 {
		return d
	}
	return 0
}

// fail records a wrong password.
func (l *attemptLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a := l.byKey[key]
	if a == nil {
		if len(l.byKey) >= maxLimiterKey {
			l.prune(now)
		}
		a = &attempts{}
		l.byKey[key] = a
	}
	a.fails++
	a.last = now
	if over := a.fails - freeAttempts; over >= 0 {
		lock := firstLockout
		for i := 0; i < over && lock < maxLockout; i++ {
			lock *= 2
		}
		if lock > maxLockout {
			lock = maxLockout
		}
		a.until = now.Add(lock)
	}
}

// ok records a correct password.
func (l *attemptLimiter) ok(key string) {
	l.mu.Lock()
	delete(l.byKey, key)
	l.mu.Unlock()
}

// prune drops stale entries; if the table is still full, it is cleared
// rather than left to grow. Called with l.mu held.
func (l *attemptLimiter) prune(now time.Time) {
	for k, a := range l.byKey {
		if now.Sub(a.last) > forgetAfter {
			delete(l.byKey, k)
		}
	}
	if len(l.byKey) >= maxLimiterKey {
		l.byKey = make(map[string]*attempts)
	}
}
