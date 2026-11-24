package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalHostName(t *testing.T) {
	allowed := []string{"pilot.example.com", "*.home.example.org", ".tail1234.ts.net"}
	cases := []struct {
		host string
		want bool
	}{
		{"192.168.1.10:8383", true},
		{"127.0.0.1", true},
		{"[::1]:8383", true},
		{"[fd00::5]", true},
		{"localhost:8383", true},
		{"pilot:8383", true},
		{"NAS", true},
		{"nas.local", true},
		{"media.lan:8383", true},
		{"pilot.home.arpa", true},
		{"box.internal.", true},
		{"pilot.example.com", true},
		{"Pilot.Example.Com:443", true},
		{"tv.home.example.org", true},
		{"box.tail1234.ts.net", true},
		// What a rebinding page looks like, and near misses.
		{"evil.example.com:8383", false},
		{"a1b2.rebind.attacker.net", false},
		{"192.168.1.10.attacker.net", false},
		{"localhost.attacker.net", false},
		{"notpilot.example.com", false},
		{"home.example.org", false},
		{"evil-lan.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := localHostName(c.host, allowed); got != c.want {
			t.Errorf("localHostName(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// A DNS-rebinding page reaches Pilot from the visitor's own browser: a
// local address, "same-origin", but under the attacker's domain.
func rebindRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "192.168.1.20:5000"
	r.Host = "evil.example.com:8383"
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Origin", "http://evil.example.com:8383")
	return r
}

func TestMiddleware_RefusesRebinding(t *testing.T) {
	a := NewAuth(testKey, AuthSettings{})
	reached := false
	h := a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, rebindRequest(method, "/api/v1/system/backup"))
		if reached || w.Code != http.StatusUnauthorized {
			t.Fatalf("%s under an attacker's host name: code %d, reached=%v", method, w.Code, reached)
		}
		if !strings.Contains(w.Body.String(), "allowed_hosts") {
			t.Errorf("the error should say how to allow a name: %s", w.Body.String())
		}
	}

	// The same browser on the server's address still needs no login.
	r := rebindRequest(http.MethodGet, "/api/v1/x")
	r.Host = "192.168.1.10:8383"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !reached {
		t.Fatalf("LAN request by IP should pass, got %d", w.Code)
	}
}

// API keys and sessions don't depend on the host name: a reverse proxy
// with its own domain keeps working for anyone who logs in.
func TestMiddleware_HostCheckOnlyGuardsNoLoginAccess(t *testing.T) {
	a := NewAuth(testKey, testSettings(t, AuthDisabledForLocal))
	ok := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	r := rebindRequest(http.MethodGet, "/api/v1/x")
	r.Header.Set("X-Api-Key", testKey)
	w := httptest.NewRecorder()
	ok.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("API key under another host name: %d", w.Code)
	}

	token, err := a.issueToken()
	if err != nil {
		t.Fatal(err)
	}
	r = rebindRequest(http.MethodGet, "/api/v1/x")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	w = httptest.NewRecorder()
	ok.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("session under another host name: %d", w.Code)
	}

	// And a listed name opens without a login.
	a.SetAllowedHosts([]string{"evil.example.com"})
	w = httptest.NewRecorder()
	ok.ServeHTTP(w, rebindRequest(http.MethodGet, "/api/v1/x"))
	if w.Code != http.StatusNoContent {
		t.Errorf("listed host: %d", w.Code)
	}
}

func TestStatus_ExplainsUntrustedHost(t *testing.T) {
	a := NewAuth(testKey, AuthSettings{})
	w := httptest.NewRecorder()
	a.StatusHandler()(w, rebindRequest(http.MethodGet, "/api/v1/auth/status"))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["authenticated"] != false || body["untrusted_host"] != "evil.example.com" {
		t.Errorf("status %v", body)
	}
}
