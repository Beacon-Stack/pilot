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

	"github.com/beacon-stack/pilot/internal/api/middleware"
	"github.com/beacon-stack/pilot/internal/api/ws"
	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/backup"
	"github.com/beacon-stack/pilot/internal/core/health"
	"github.com/beacon-stack/pilot/internal/core/importer"
	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	"github.com/beacon-stack/pilot/internal/core/recyclebin"
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	"github.com/beacon-stack/pilot/internal/core/remotepath"
	"github.com/beacon-stack/pilot/internal/core/seriesbulk"
	pilotdb "github.com/beacon-stack/pilot/internal/db"
	dbgen "github.com/beacon-stack/pilot/internal/db/generated"
)

const routerTestKey = "f00dfacef00dfacef00dfacef00dfacef00dfacef00dfacef00dfacef00dface"

type testEnv struct {
	srv        *httptest.Server
	dbPath     string
	configPath string
	pulseCalls *atomic.Int32
}

const (
	routerTestUser = "admin"
	routerTestPass = "correct horse battery"
)

// strictSettings requires a login from every address. httptest clients
// connect from 127.0.0.1, so the default (disabled_for_local) would let
// them straight in; the strict tests below use this instead.
func strictSettings(t *testing.T) middleware.AuthSettings {
	t.Helper()
	old := middleware.PasswordIterations
	middleware.PasswordIterations = 1000
	t.Cleanup(func() { middleware.PasswordIterations = old })
	hash, err := middleware.HashPassword(routerTestPass)
	if err != nil {
		t.Fatal(err)
	}
	return middleware.AuthSettings{Required: middleware.AuthEnabled, Username: routerTestUser, PasswordHash: hash}
}

func newTestEnv(t *testing.T, apiKey string) *testEnv {
	t.Helper()
	return newTestEnvWith(t, apiKey, strictSettings(t))
}

func newTestEnvWith(t *testing.T, apiKey string, settings middleware.AuthSettings) *testEnv {
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
		Auth:                   config.Secret(apiKey),
		AuthSettings:           settings,
		ConfigFile:             filepath.Join(dir, "config.yaml"),
		Logger:                 logger,
		StartTime:              time.Now(),
		DB:                     db,
		DBType:                 "sqlite",
		DBPath:                 dbPath,
		WSHub:                  ws.NewHub(logger),
		ReleaseProfileService:  releaseprofile.NewService(dbgen.New(db)),
		Queries:                dbgen.New(db),
		HealthService:          health.NewService(dbgen.New(db), nil, nil),
		RemotePathService:      remotepath.NewService(dbgen.New(db)),
		BackupService:          backup.NewService(db, dbgen.New(db), dbPath),
		MediaManagementService: mediamanagement.NewService(dbgen.New(db)),
		RecycleBinService:      recyclebin.New(mediamanagement.NewService(dbgen.New(db)), logger),
		SeriesBulkService:      seriesbulk.New(db, nil, nil, logger),
		ImporterService:        importer.NewService(dbgen.New(db), nil, logger, mediamanagement.NewService(dbgen.New(db)), nil, nil),
		PulseSyncHandler: func(w http.ResponseWriter, _ *http.Request) {
			pulseCalls.Add(1)
			w.WriteHeader(http.StatusAccepted)
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, dbPath: dbPath, configPath: filepath.Join(dir, "config.yaml"), pulseCalls: &pulseCalls}
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
	{"POST", "/api/v1/recycle-bin/empty"},
	{"PUT", "/api/v1/series/bulk"},
	{"POST", "/api/v1/series/bulk/delete"},
	{"POST", "/api/v1/manual-import/scan"},
	{"POST", "/api/v1/manual-import"},
	{"PUT", "/api/v1/media-management"},
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

func login(t *testing.T, e *testEnv, password string) (result, *http.Cookie) {
	t.Helper()
	resp, _ := e.do(t, "POST", "/api/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"},
		`{"username":"`+routerTestUser+`","password":"`+password+`"}`)
	for _, c := range resp.Cookies() {
		if c.Name == "pilot_session" {
			return resp, c
		}
	}
	return resp, nil
}

func TestAuth_LoginCookieAuthenticates(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, c := login(t, e, routerTestPass)
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

func TestAuth_LoginWrongPassword(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, c := login(t, e, "nope")
	if resp.StatusCode != http.StatusUnauthorized || c != nil {
		t.Fatalf("status = %d cookie = %v, want 401 and no cookie", resp.StatusCode, c)
	}
}

func TestAuth_LoginCrossSiteRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	resp, _ := e.do(t, "POST", "/api/v1/auth/login",
		map[string]string{"Sec-Fetch-Site": "cross-site"}, `{"username":"`+routerTestUser+`","password":"`+routerTestPass+`"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAuth_CookieCrossSiteRejected(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	_, c := login(t, e, routerTestPass)
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
	_, oldCookie := login(t, old, routerTestPass)

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
	_, c := login(t, e, routerTestPass)
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

// ── Sonarr-style default: no login on your own network ────────────────────

// remote makes a request look like it came from the internet through a
// local reverse proxy (the test client itself is on 127.0.0.1).
var remote = map[string]string{"X-Forwarded-For": "203.0.113.9", "Sec-Fetch-Site": "same-origin"}

func TestAuth_DefaultLocalNeedsNoLogin(t *testing.T) {
	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}
	r, _ := e.do(t, "GET", "/api/v1/system/status", sameOrigin, "")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("local GET with no credentials: status = %d, want 200", r.StatusCode)
	}
	r, _ = e.do(t, "POST", "/api/v1/hooks/pulse/sync", sameOrigin, "")
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("local POST with no credentials: status = %d, want 202", r.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/v1/ws"
	status, conn := wsDial(ctx, wsURL, http.Header{"Sec-Fetch-Site": {"same-origin"}})
	if conn == nil {
		t.Fatalf("local ws with no credentials failed: status = %d", status)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

func TestAuth_DefaultRemoteStillNeedsLogin(t *testing.T) {
	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	for _, ep := range protectedEndpoints {
		resp, body := e.do(t, ep.method, ep.path, remote, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s from the internet: status = %d, want 401", ep.method, ep.path, resp.StatusCode)
		}
		if strings.Contains(body, routerTestKey) {
			t.Errorf("%s %s leaked the API key", ep.method, ep.path)
		}
	}
	if e.pulseCalls.Load() != 0 {
		t.Fatal("remote request reached the pulse handler")
	}
}

// A malicious web page open in a browser on the LAN must not be able to
// drive the API through the local bypass.
func TestAuth_DefaultLocalCrossSiteRejected(t *testing.T) {
	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	resp, _ := e.do(t, "POST", "/api/v1/hooks/pulse/sync", map[string]string{"Sec-Fetch-Site": "cross-site"}, "")
	if resp.StatusCode != http.StatusForbidden || e.pulseCalls.Load() != 0 {
		t.Fatalf("cross-site POST via local bypass: status = %d calls = %d, want 403 and 0", resp.StatusCode, e.pulseCalls.Load())
	}
	resp, body := e.do(t, "GET", "/api/v1/system/config/apikey", map[string]string{"Sec-Fetch-Site": "cross-site"}, "")
	if resp.StatusCode != http.StatusForbidden || strings.Contains(body, routerTestKey) {
		t.Fatalf("cross-site GET via local bypass: status = %d, want 403 without the key", resp.StatusCode)
	}
}

func TestAuth_Status(t *testing.T) {
	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	_, body := e.do(t, "GET", "/api/v1/auth/status", map[string]string{"Sec-Fetch-Site": "same-origin"}, "")
	if !strings.Contains(body, `"authenticated":true`) || !strings.Contains(body, `"method":"local"`) ||
		!strings.Contains(body, `"credentials_configured":false`) {
		t.Fatalf("local status = %s", body)
	}
	_, body = e.do(t, "GET", "/api/v1/auth/status", remote, "")
	if !strings.Contains(body, `"authenticated":false`) {
		t.Fatalf("remote status = %s", body)
	}

	strict := newTestEnv(t, routerTestKey)
	_, body = strict.do(t, "GET", "/api/v1/auth/status", nil, "")
	if !strings.Contains(body, `"authenticated":false`) || !strings.Contains(body, `"credentials_configured":true`) {
		t.Fatalf("strict status = %s", body)
	}
}

func TestAuth_LoginWithoutCredentialsConfigured(t *testing.T) {
	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	resp, _ := e.do(t, "POST", "/api/v1/auth/login", remote, `{"username":"admin","password":"anything"}`)
	if resp.StatusCode != http.StatusUnauthorized || len(resp.Cookies()) != 0 {
		t.Fatalf("status = %d, want 401 with no cookie", resp.StatusCode)
	}
	// The API key is not a login password.
	resp, _ = e.do(t, "POST", "/api/v1/auth/login", remote, `{"username":"admin","password":"`+routerTestKey+`"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("API key as password: status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_SettingsFlow(t *testing.T) {
	old := middleware.PasswordIterations
	middleware.PasswordIterations = 1000
	t.Cleanup(func() { middleware.PasswordIterations = old })

	e := newTestEnvWith(t, routerTestKey, middleware.AuthSettings{})
	local := map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}

	// Can't require a login everywhere before one exists.
	r, _ := e.do(t, "PUT", "/api/v1/system/auth", local, `{"required":"enabled","username":""}`)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("enable without login: status = %d, want 400", r.StatusCode)
	}
	r, _ = e.do(t, "PUT", "/api/v1/system/auth", local, `{"required":"disabled_for_local","username":"me","password":"short"}`)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: status = %d, want 400", r.StatusCode)
	}

	// Set a login; LAN still needs none, the internet can now log in.
	r, body := e.do(t, "PUT", "/api/v1/system/auth", local, `{"required":"disabled_for_local","username":"me","password":"long enough pw"}`)
	if r.StatusCode != http.StatusOK || strings.Contains(body, "pbkdf2") {
		t.Fatalf("set login: status = %d body = %s", r.StatusCode, body)
	}
	raw, _ := os.ReadFile(e.configPath)
	if !strings.Contains(string(raw), "pbkdf2-sha256$") || strings.Contains(string(raw), "long enough pw") {
		t.Fatalf("config.yaml should hold the hash, not the password:\n%s", raw)
	}
	if r, _ := e.do(t, "GET", "/api/v1/system/status", local, ""); r.StatusCode != http.StatusOK {
		t.Fatalf("LAN after setting login: status = %d, want 200", r.StatusCode)
	}
	r, _ = e.do(t, "POST", "/api/v1/auth/login", remote, `{"username":"me","password":"long enough pw"}`)
	var cookie *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == middleware.SessionCookieName {
			cookie = c
		}
	}
	if r.StatusCode != http.StatusNoContent || cookie == nil {
		t.Fatalf("remote login: status = %d", r.StatusCode)
	}
	withCookie := map[string]string{"X-Forwarded-For": "203.0.113.9", "Sec-Fetch-Site": "same-origin", "Cookie": cookie.Name + "=" + cookie.Value}
	if r, _ := e.do(t, "GET", "/api/v1/system/status", withCookie, ""); r.StatusCode != http.StatusOK {
		t.Fatalf("remote with session: status = %d, want 200", r.StatusCode)
	}

	// Require login everywhere (keeping the password); LAN now needs it too.
	r, _ = e.do(t, "PUT", "/api/v1/system/auth", local, `{"required":"enabled","username":"me"}`)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("enable: status = %d", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/api/v1/system/status", local, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("LAN with required=enabled: status = %d, want 401", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/api/v1/system/status", withCookie, ""); r.StatusCode != http.StatusOK {
		t.Fatalf("session after enabling (same password): status = %d, want 200", r.StatusCode)
	}

	// Changing the password logs existing sessions out.
	apiKey := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}
	r, _ = e.do(t, "PUT", "/api/v1/system/auth", apiKey, `{"required":"enabled","username":"me","password":"a new password"}`)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("change password: status = %d", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/api/v1/system/status", withCookie, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old session after password change: status = %d, want 401", r.StatusCode)
	}
}
