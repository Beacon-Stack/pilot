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
	a := NewAuth(testKey, AuthSettings{})
	tok, err := a.issueToken()
	if err != nil {
		t.Fatal(err)
	}
	if !a.verifyToken(tok) {
		t.Fatal("freshly issued token did not verify")
	}
}

func TestToken_Expired(t *testing.T) {
	a := NewAuth(testKey, AuthSettings{})
	tok, _ := a.issueToken()
	a.now = func() time.Time { return time.Now().Add(DefaultSessionTTL + time.Minute) }
	if a.verifyToken(tok) {
		t.Fatal("expired token verified")
	}
}

func TestToken_Tampered(t *testing.T) {
	a := NewAuth(testKey, AuthSettings{})
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
	tok, _ := NewAuth(testKey, AuthSettings{}).issueToken()
	if NewAuth("a-different-api-key-entirely-xxxx", AuthSettings{}).verifyToken(tok) {
		t.Fatal("token signed under the old API key verified under the new one")
	}
}

func TestEmptyAPIKeyFailsClosed(t *testing.T) {
	a := NewAuth("", AuthSettings{})
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
	a := NewAuth(testKey, testSettings(t, AuthEnabled))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"correct horse"}`))
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	a.LoginHandler()(w, r)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected one Secure cookie behind TLS proxy, got %+v", cookies)
	}
}

func testSettings(t *testing.T, required string) AuthSettings {
	t.Helper()
	old := PasswordIterations
	PasswordIterations = 1000
	t.Cleanup(func() { PasswordIterations = old })
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	return AuthSettings{Required: required, Username: "admin", PasswordHash: hash}
}

func TestToken_PasswordChangeInvalidatesSessions(t *testing.T) {
	a := NewAuth(testKey, testSettings(t, AuthEnabled))
	tok, _ := a.issueToken()
	a.SetSettings(testSettings(t, AuthEnabled)) // new salt → new hash
	if a.verifyToken(tok) {
		t.Fatal("session survived a password change")
	}
}

func TestPassword_RoundTrip(t *testing.T) {
	s := testSettings(t, AuthEnabled)
	if !CheckPassword(s.PasswordHash, "correct horse") {
		t.Fatal("correct password rejected")
	}
	for _, bad := range []string{"", "correct horse ", "Correct horse"} {
		if CheckPassword(s.PasswordHash, bad) {
			t.Fatalf("wrong password %q accepted", bad)
		}
	}
	for _, h := range []string{"", "plain", "pbkdf2-sha256$x$a$b", "pbkdf2-sha256$99999999999$AA$AA", "md5$1$AA$AA"} {
		if CheckPassword(h, "correct horse") {
			t.Fatalf("malformed hash %q matched", h)
		}
	}
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password hashed")
	}
}

func TestClientIsLocal(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		hdr    map[string]string
		want   bool
	}{
		{"loopback v4", "127.0.0.1:5000", nil, true},
		{"loopback v6", "[::1]:5000", nil, true},
		{"lan 192.168", "192.168.1.20:5000", nil, true},
		{"lan 10/8", "10.0.0.5:5000", nil, true},
		{"docker bridge", "172.18.0.1:5000", nil, true},
		{"ula v6", "[fd12:3456::1]:5000", nil, true},
		{"link-local", "169.254.3.4:5000", nil, true},
		{"v4-mapped lan", "[::ffff:192.168.1.20]:5000", nil, true},
		{"public peer", "203.0.113.9:5000", nil, false},
		{"public peer claiming lan", "203.0.113.9:5000", map[string]string{"X-Forwarded-For": "192.168.1.2"}, false},
		{"cgnat is not local", "100.64.0.1:5000", nil, false},
		{"local proxy, lan client", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "192.168.1.20"}, true},
		{"local proxy, internet client", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "203.0.113.9"}, false},
		{"local proxy, chain with internet hop", "172.18.0.2:5000", map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.1"}, false},
		{"local proxy, x-real-ip internet", "172.18.0.2:5000", map[string]string{"X-Real-Ip": "198.51.100.4"}, false},
		{"cloudflared", "172.18.0.2:5000", map[string]string{"Cf-Connecting-Ip": "198.51.100.4"}, false},
		{"rfc7239", "172.18.0.2:5000", map[string]string{"Forwarded": `for="[2001:db8::1]:443";proto=https`}, false},
		{"rfc7239 lan", "172.18.0.2:5000", map[string]string{"Forwarded": "for=192.168.1.5"}, true},
		{"garbage forwarded", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "unknown"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
			r.RemoteAddr = c.remote
			for k, v := range c.hdr {
				r.Header.Set(k, v)
			}
			if got := clientIsLocal(r); got != c.want {
				t.Fatalf("clientIsLocal = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAuthenticate_LocalBypass(t *testing.T) {
	req := func(remote string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
		r.RemoteAddr = remote
		return r
	}
	// Default: local in, remote out.
	a := NewAuth(testKey, AuthSettings{})
	if a.authenticate(req("192.168.1.20:1")) != authLocal {
		t.Fatal("default settings: LAN request should need no login")
	}
	if a.authenticate(req("203.0.113.9:1")) != authNone {
		t.Fatal("default settings: internet request must need a login")
	}
	// Enabled with credentials: local must log in too.
	a = NewAuth(testKey, testSettings(t, AuthEnabled))
	if a.authenticate(req("127.0.0.1:1")) != authNone {
		t.Fatal("required=enabled: local request must need a login")
	}
	// Enabled without credentials: don't lock the owner out.
	a = NewAuth(testKey, AuthSettings{Required: AuthEnabled})
	if a.authenticate(req("127.0.0.1:1")) != authLocal {
		t.Fatal("required=enabled with no login set must still let local in")
	}
	// A wrong API key is a hard no, even from the LAN.
	r := req("192.168.1.20:1")
	r.Header.Set("X-Api-Key", "wrong")
	if a.authenticate(r) != authNone {
		t.Fatal("wrong X-Api-Key from LAN accepted")
	}
}
