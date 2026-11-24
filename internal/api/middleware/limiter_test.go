package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAttemptLimiter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := newAttemptLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < freeAttempts-1; i++ {
		l.fail("a")
	}
	if l.wait("a") != 0 {
		t.Fatal("the first few misses shouldn't lock anything")
	}
	l.fail("a")
	if d := l.wait("a"); d != firstLockout {
		t.Fatalf("after %d misses wait = %v, want %v", freeAttempts, d, firstLockout)
	}
	if l.wait("b") != 0 {
		t.Fatal("another address must not be held up")
	}
	now = now.Add(firstLockout)
	if l.wait("a") != 0 {
		t.Fatal("the lock should have run out")
	}
	l.fail("a")
	if d := l.wait("a"); d != 2*firstLockout {
		t.Fatalf("the next miss should double the wait, got %v", d)
	}
	for i := 0; i < 20; i++ {
		l.fail("a")
	}
	if d := l.wait("a"); d != maxLockout {
		t.Fatalf("wait is capped at %v, got %v", maxLockout, d)
	}
	l.ok("a")
	if l.wait("a") != 0 {
		t.Fatal("a correct password clears the count")
	}
	// Old misses are forgotten.
	for i := 0; i < freeAttempts; i++ {
		l.fail("c")
	}
	now = now.Add(forgetAfter + time.Second)
	if l.wait("c") != 0 {
		t.Fatal("misses older than forgetAfter shouldn't count")
	}
}

func TestLogin_LocksAfterRepeatedMisses(t *testing.T) {
	old := PasswordIterations
	PasswordIterations = 1000
	t.Cleanup(func() { PasswordIterations = old })

	a := NewAuth(testKey, testSettings(t, AuthEnabled))
	login := func(remote, password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		a.LoginHandler()(w, r)
		return w
	}
	for i := 0; i < freeAttempts; i++ {
		if w := login("203.0.113.9:1000", "guess"); w.Code != http.StatusUnauthorized {
			t.Fatalf("miss %d: %d", i, w.Code)
		}
	}
	// Locked now, even with the right password, and from any port.
	w := login("203.0.113.9:2000", "correct horse")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("after %d misses: %d, Retry-After %q", freeAttempts, w.Code, w.Header().Get("Retry-After"))
	}
	// Someone else is not affected.
	if w := login("198.51.100.4:1", "correct horse"); w.Code != http.StatusNoContent {
		t.Fatalf("another address: %d", w.Code)
	}
}
