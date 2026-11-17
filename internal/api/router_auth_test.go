package api

// Regression suite for the Sec-Fetch-Site auth bypass.
//
// Before this fix, any client that sent "Sec-Fetch-Site: same-origin" was
// treated as an authenticated browser, so
//
//	curl -H 'Sec-Fetch-Site: same-origin' http://host:8383/api/v1/system/config/apikey
//
// returned the full API key. These tests drive the real router and must
// keep passing: if TestAuth_SecFetchSiteIsNotACredential fails, the bypass
// is back. Fix the router or internal/api/middleware/auth.go, not the test.

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	_ "modernc.org/sqlite"

	"github.com/beacon-stack/pilot/internal/api/ws"
	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/backup"
	"github.com/beacon-stack/pilot/internal/core/health"
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	"github.com/beacon-stack/pilot/internal/core/remotepath"
	pilotdb "github.com/beacon-stack/pilot/internal/db"
	dbgen "github.com/beacon-stack/pilot/internal/db/generated"
)

const routerTestKey = "f00dfacef00dfacef00dfacef00dfacef00dfacef00dfacef00dfacef00dface"

type testEnv struct {
	srv        *httptest.Server
	dbPath     string
	pulseCalls *atomic.Int32
}

func newTestEnv(t *testing.T, apiKey string) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pilot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := pilotdb.Migrate(db); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var pulseCalls atomic.Int32
	h := NewRouter(RouterConfig{
		Auth:                  config.Secret(apiKey),
		Logger:                logger,
		StartTime:             time.Now(),
		DB:                    db,
		DBType:                "sqlite",
		DBPath:                dbPath,
		WSHub:                 ws.NewHub(logger),
		ReleaseProfileService: releaseprofile.NewService(dbgen.New(db)),
		Queries:               dbgen.New(db),
		HealthService:         health.NewService(dbgen.New(db), nil, nil),
		RemotePathService:     remotepath.NewService(dbgen.New(db)),
		BackupService:         backup.NewService(db, dbgen.New(db), dbPath),
		PulseSyncHandler: func(w http.ResponseWriter, _ *http.Request) {
			pulseCalls.Add(1)
			w.WriteHeader(http.StatusAccepted)
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, dbPath: dbPath, pulseCalls: &pulseCalls}
}

// result is the bits of a response the tests inspect. The body is already
// read and closed by do().
type result struct {
	StatusCode int
	cookies    []*http.Cookie
}

func (r result) Cookies() []*http.Cookie { return r.cookies }

func (e *testEnv) do(t *testing.T, method, path string, hdr map[string]string, body string) (result, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return result{StatusCode: resp.StatusCode, cookies: resp.Cookies()}, string(b)
}

// wsDial attempts a WebSocket handshake and returns the HTTP status of
// the handshake response (101 on success) plus the conn, if any.
func wsDial(ctx context.Context, url string, hdr http.Header) (int, *websocket.Conn) {
	conn, resp, _ := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	return status, conn
}

// Every protected surface the old code exposed through the header bypass.
var protectedEndpoints = []struct{ method, path string }{
	{"GET", "/api/v1/system/config/apikey"},
	{"GET", "/api/v1/system/config"},
	{"GET", "/api/v1/system/env"},
	{"GET", "/api/v1/fs/browse?path=/"},
	{"GET", "/api/v1/system/backup"},
	{"POST", "/api/v1/system/restore"},
	{"GET", "/api/v1/ws"},
	{"POST", "/api/v1/hooks/pulse/sync"},
	{"GET", "/api/v1/release-profiles"},
	{"GET", "/api/v1/delay-profile"},
	{"PUT", "/api/v1/delay-profile"},
	{"GET", "/api/v1/queue/pending"},
	{"GET", "/api/v1/language-preference"},
	{"PUT", "/api/v1/language-preference"},
	{"GET", "/api/v1/system/health"},
	{"GET", "/api/v1/remote-path-mappings"},
	{"GET", "/api/v1/system/backups"},
	{"POST", "/api/v1/system/backups"},
	{"GET", "/api/v1/system/backups/pilot-manual-20260101-000000.db/download"},
	{"POST", "/api/v1/system/backups/pilot-manual-20260101-000000.db/restore"},
	{"DELETE", "/api/v1/system/backups/pilot-manual-20260101-000000.db"},
	{"GET", "/api/v1/system/backup-settings"},
	{"PUT", "/api/v1/system/backup-settings"},
	{"POST", "/api/v1/remote-path-mappings"},
	{"DELETE", "/api/v1/remote-path-mappings/x"},
	{"POST", "/api/v1/system/health/check"},
	{"POST", "/api/v1/release-profiles"},
	{"GET", "/api/openapi.json"},
}

func TestAuth_SecFetchSiteIsNotACredential(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	for _, ep := range protectedEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			resp, body := e.do(t, ep.method, ep.path, map[string]string{"Sec-Fetch-Site": "same-origin"}, "SQLite format 3\x00 padding......")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (Sec-Fetch-Site must not authenticate). body: %s", resp.StatusCode, body)
			}
			if strings.Contains(body, routerTestKey) {
				t.Fatal("response leaked the API key")
			}
		})
	}
	if n := e.pulseCalls.Load(); n != 0 {
		t.Fatalf("pulse sync handler ran %d times without auth", n)
	}
	if _, err := os.Stat(e.dbPath + ".restore"); !os.IsNotExist(err) {
		t.Fatal("unauthenticated restore wrote a staging file")
	}
}

func TestAuth_NoCredentialsRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	for _, ep := range protectedEndpoints {
		resp, _ := e.do(t, ep.method, ep.path, nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", ep.method, ep.path, resp.StatusCode)
		}
	}
}

func TestAuth_WrongAPIKeyRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, _ := e.do(t, "GET", "/api/v1/system/config/apikey", map[string]string{"X-Api-Key": "wrong", "Sec-Fetch-Site": "same-origin"}, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_APIKeyAccepted(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, body := e.do(t, "GET", "/api/v1/system/config/apikey", map[string]string{"X-Api-Key": routerTestKey}, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, routerTestKey) {
		t.Fatalf("status = %d body = %s, want 200 with key", resp.StatusCode, body)
	}
	resp, _ = e.do(t, "POST", "/api/v1/hooks/pulse/sync", map[string]string{"X-Api-Key": routerTestKey}, "")
	if resp.StatusCode != http.StatusAccepted || e.pulseCalls.Load() != 1 {
		t.Fatalf("pulse hook with valid key: status = %d calls = %d", resp.StatusCode, e.pulseCalls.Load())
	}
}

func TestAuth_PublicRoutes(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	if resp, _ := e.do(t, "GET", "/health", nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", resp.StatusCode)
	}
}

// Path variants must never reach a protected handler without auth. They
// may hit the SPA fallback (200 with index.html), but must not leak data.
func TestAuth_PathVariantsDoNotBypass(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	for _, p := range []string{
		"//api/v1/system/config/apikey",
		"/api/v1/system/config/apikey/",
		"/api//v1/system/config/apikey",
		"/API/v1/system/config/apikey",
		"/%61pi/v1/system/config/apikey",
		"/api/v1/system/config/apikey?x=../../health",
		"/./api/v1/system/config/apikey",
	} {
		resp, body := e.do(t, "GET", p, map[string]string{"Sec-Fetch-Site": "same-origin"}, "")
		if strings.Contains(body, routerTestKey) {
			t.Errorf("%s: leaked the API key (status %d)", p, resp.StatusCode)
		}
	}
}

func login(t *testing.T, e *testEnv, key string) (result, *http.Cookie) {
	t.Helper()
	resp, _ := e.do(t, "POST", "/api/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"},
		`{"api_key":"`+key+`"}`)
	for _, c := range resp.Cookies() {
		if c.Name == "pilot_session" {
			return resp, c
		}
	}
	return resp, nil
}

func TestAuth_LoginCookieAuthenticates(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, c := login(t, e, routerTestKey)
	if resp.StatusCode != http.StatusNoContent || c == nil {
		t.Fatalf("login status = %d cookie = %v", resp.StatusCode, c)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie flags wrong: %+v", c)
	}

	cookieHdr := map[string]string{"Cookie": c.Name + "=" + c.Value, "Sec-Fetch-Site": "same-origin"}
	r, body := e.do(t, "GET", "/api/v1/system/config/apikey", cookieHdr, "")
	if r.StatusCode != http.StatusOK || !strings.Contains(body, routerTestKey) {
		t.Fatalf("cookie GET status = %d, want 200", r.StatusCode)
	}
	r, _ = e.do(t, "POST", "/api/v1/hooks/pulse/sync", cookieHdr, "")
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("cookie POST status = %d, want 202", r.StatusCode)
	}
}

func TestAuth_LoginWrongKey(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, c := login(t, e, "nope")
	if resp.StatusCode != http.StatusUnauthorized || c != nil {
		t.Fatalf("status = %d cookie = %v, want 401 and no cookie", resp.StatusCode, c)
	}
}

func TestAuth_LoginCrossSiteRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, _ := e.do(t, "POST", "/api/v1/auth/login",
		map[string]string{"Sec-Fetch-Site": "cross-site"}, `{"api_key":"`+routerTestKey+`"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAuth_CookieCrossSiteRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	_, c := login(t, e, routerTestKey)
	for _, m := range []string{"GET", "POST"} {
		path := "/api/v1/system/config/apikey"
		if m == "POST" {
			path = "/api/v1/hooks/pulse/sync"
		}
		resp, _ := e.do(t, m, path, map[string]string{"Cookie": c.Name + "=" + c.Value, "Sec-Fetch-Site": "cross-site"}, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s cross-site with cookie: status = %d, want 403", m, resp.StatusCode)
		}
	}
	if e.pulseCalls.Load() != 0 {
		t.Fatal("cross-site POST reached the handler")
	}
}

func TestAuth_ForgedAndStaleCookiesRejected(t *testing.T) {
	old := newTestEnv(t, "old-key-old-key-old-key-old-key-old-key")
	_, oldCookie := login(t, old, "old-key-old-key-old-key-old-key-old-key")

	e := newTestEnv(t, routerTestKey)
	for name, v := range map[string]string{
		"rotated api key": oldCookie.Value,
		"garbage":         "v1.99999999999.AAAA.forged",
		"empty":           "",
	} {
		resp, _ := e.do(t, "GET", "/api/v1/system/config/apikey",
			map[string]string{"Cookie": "pilot_session=" + v, "Sec-Fetch-Site": "same-origin"}, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, resp.StatusCode)
		}
	}
}

func TestAuth_LogoutClearsCookie(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, _ := e.do(t, "POST", "/api/v1/auth/logout", map[string]string{"Sec-Fetch-Site": "same-origin"}, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	cs := resp.Cookies()
	if len(cs) != 1 || cs[0].Name != "pilot_session" || cs[0].MaxAge >= 0 {
		t.Fatalf("logout did not expire the cookie: %+v", cs)
	}
}

func TestAuth_WebSocket(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/v1/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Header bypass: rejected.
	if status, _ := wsDial(ctx, wsURL, http.Header{"Sec-Fetch-Site": {"same-origin"}}); status != http.StatusUnauthorized {
		t.Fatalf("ws with only Sec-Fetch-Site: status = %d, want 401", status)
	}

	// Session cookie: accepted.
	_, c := login(t, e, routerTestKey)
	cookie := c.Name + "=" + c.Value
	status, conn := wsDial(ctx, wsURL, http.Header{"Cookie": {cookie}, "Sec-Fetch-Site": {"same-origin"}})
	if conn == nil {
		t.Fatalf("ws with session cookie failed: status = %d", status)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")

	// Cross-site WebSocket hijack with a valid cookie: rejected.
	if status, _ := wsDial(ctx, wsURL, http.Header{"Cookie": {cookie}, "Sec-Fetch-Site": {"cross-site"}}); status != http.StatusForbidden {
		t.Fatalf("cross-site ws with cookie: status = %d, want 403", status)
	}
}
