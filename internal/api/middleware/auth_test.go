package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef"

func TestToken_RoundTrip(t *testing.T) {
	a := NewAuth(testKey)
	tok, err := a.issueToken()
	if err != nil {
		t.Fatal(err)
	}
	if !a.verifyToken(tok) {
		t.Fatal("freshly issued token did not verify")
	}
}

func TestToken_Expired(t *testing.T) {
	a := NewAuth(testKey)
	tok, _ := a.issueToken()
	a.now = func() time.Time { return time.Now().Add(DefaultSessionTTL + time.Minute) }
	if a.verifyToken(tok) {
		t.Fatal("expired token verified")
	}
}

func TestToken_Tampered(t *testing.T) {
	a := NewAuth(testKey)
	tok, _ := a.issueToken()
	parts := strings.Split(tok, ".")
	// Push the expiry far into the future without re-signing.
	parts[1] = "99999999999"
	if a.verifyToken(strings.Join(parts, ".")) {
		t.Fatal("token with modified expiry verified")
	}
	for _, bad := range []string{"", ".", "v1", "v1.1.2", tok + "x", "x" + tok} {
		if a.verifyToken(bad) {
			t.Fatalf("malformed token %q verified", bad)
		}
	}
}

func TestToken_RotatedAPIKeyInvalidatesSessions(t *testing.T) {
	tok, _ := NewAuth(testKey).issueToken()
	if NewAuth("a-different-api-key-entirely-xxxx").verifyToken(tok) {
		t.Fatal("token signed under the old API key verified under the new one")
	}
}

func TestEmptyAPIKeyFailsClosed(t *testing.T) {
	a := NewAuth("")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.Header.Set("X-Api-Key", "")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if a.authenticate(r) != authNone {
		t.Fatal("empty API key config must reject everything")
	}
	tok, _ := a.issueToken()
	if a.verifyToken(tok) {
		t.Fatal("empty API key config must not accept sessions")
	}
}

func TestCrossSiteSafe(t *testing.T) {
	cases := []struct {
		name, method, sfs, origin string
		want                      bool
	}{
		{"same-origin GET", "GET", "same-origin", "", true},
		{"same-origin POST", "POST", "same-origin", "", true},
		{"user navigation", "GET", "none", "", true},
		{"cross-site GET", "GET", "cross-site", "", false},
		{"cross-site POST", "POST", "cross-site", "", false},
		{"same-site POST", "POST", "same-site", "", false},
		{"unknown value", "POST", "bogus", "", false},
		{"legacy GET", "GET", "", "http://evil.example", true},
		{"legacy POST no origin", "POST", "", "", true},
		{"legacy POST same origin", "POST", "", "http://pilot.test", true},
		{"legacy POST foreign origin", "POST", "", "http://evil.example", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "http://pilot.test/api/v1/x", nil)
			if c.sfs != "" {
				r.Header.Set("Sec-Fetch-Site", c.sfs)
			}
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if got := crossSiteSafe(r); got != c.want {
				t.Fatalf("crossSiteSafe = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLogin_SecureFlagBehindTLSProxy(t *testing.T) {
	a := NewAuth(testKey)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"api_key":"`+testKey+`"}`))
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	a.LoginHandler()(w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected one Secure cookie behind TLS proxy, got %+v", cookies)
	}
}
