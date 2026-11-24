package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SessionCookieName is the cookie that carries a browser session.
const SessionCookieName = "pilot_session"

// DefaultSessionTTL is how long a browser session stays valid after login.
const DefaultSessionTTL = 30 * 24 * time.Hour

// sessionVersion prefixes every token so the format can change later
// without old tokens being misparsed.
const sessionVersion = "v1"

// When a login is required, mirroring Sonarr's "Authentication Required".
const (
	// AuthDisabledForLocal lets requests from local addresses through
	// without credentials; everyone else must log in. The default.
	AuthDisabledForLocal = "disabled_for_local"
	// AuthEnabled requires a login from every address.
	AuthEnabled = "enabled"
)

// AuthSettings is the user-editable part of authentication.
type AuthSettings struct {
	// Required is AuthDisabledForLocal or AuthEnabled. Anything else is
	// treated as AuthDisabledForLocal.
	Required string
	// Username and PasswordHash (from HashPassword) are the login for
	// the browser UI. Both empty means no login is set up yet.
	Username     string
	PasswordHash string
}

// CredentialsConfigured reports whether a username and password are set.
func (s AuthSettings) CredentialsConfigured() bool {
	return s.Username != "" && s.PasswordHash != ""
}

// LoginRequiredEverywhere reports whether local addresses must log in
// too. AuthEnabled without credentials would lock everyone out, so it
// falls back to letting local addresses through.
func (s AuthSettings) LoginRequiredEverywhere() bool {
	return s.Required == AuthEnabled && s.CredentialsConfigured()
}

// Auth authenticates API requests. A request is accepted when it carries
// any of:
//
//   - a valid X-Api-Key header (scripts, Pulse, other services),
//   - a valid session cookie issued by Login (the browser UI), or
//   - nothing, when it comes from a local address and the settings don't
//     require a login there (AuthDisabledForLocal, the default). This
//     is how Sonarr works: on your own network the UI just opens.
//
// Sec-Fetch-Site is deliberately NOT a credential: any non-browser client
// can set it to "same-origin". It is only used as a CSRF check on
// cookie and local requests, where the browser guarantees its value.
//
// Sessions are stateless HMAC-signed tokens. The signing key is derived
// from the API key and the password hash, so rotating the API key or
// changing the password invalidates every session.
type Auth struct {
	apiKey  []byte
	ttl     time.Duration
	now     func() time.Time
	limiter *attemptLimiter

	mu           sync.RWMutex
	settings     AuthSettings
	signKey      []byte
	allowedHosts []string // extra Host names trusted for local, no-login access
}

// NewAuth builds an Auth for the given API key and settings. An empty API
// key rejects every request (fail closed).
func NewAuth(apiKey string, settings AuthSettings) *Auth {
	a := &Auth{
		apiKey:  []byte(apiKey),
		ttl:     DefaultSessionTTL,
		now:     time.Now,
		limiter: newAttemptLimiter(),
	}
	a.SetSettings(settings)
	return a
}

// SetAllowedHosts sets extra host names (server.allowed_hosts) under
// which local addresses may use Pilot without logging in, besides IP
// addresses, localhost, single-word and home-network names.
func (a *Auth) SetAllowedHosts(hosts []string) {
	a.mu.Lock()
	a.allowedHosts = append([]string(nil), hosts...)
	a.mu.Unlock()
}

func (a *Auth) hostTrusted(r *http.Request) bool {
	a.mu.RLock()
	allowed := a.allowedHosts
	a.mu.RUnlock()
	return localHostName(r.Host, allowed)
}

// Settings returns the current authentication settings.
func (a *Auth) Settings() AuthSettings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

// SetSettings replaces the authentication settings. Changing the password
// hash logs out every existing session.
func (a *Auth) SetSettings(s AuthSettings) {
	if s.Required != AuthEnabled {
		s.Required = AuthDisabledForLocal
	}
	mac := hmac.New(sha256.New, a.apiKey)
	mac.Write([]byte("pilot-session-" + sessionVersion + "\x00" + s.PasswordHash))
	a.mu.Lock()
	a.settings = s
	a.signKey = mac.Sum(nil)
	a.mu.Unlock()
}

type authMethod int

const (
	authNone authMethod = iota
	authAPIKey
	authSession
	authLocal
)

// String is the method name reported by GET /api/v1/auth/status.
func (m authMethod) String() string {
	switch m {
	case authAPIKey:
		return "api_key"
	case authSession:
		return "session"
	case authLocal:
		return "local"
	default:
		return "none"
	}
}

func (a *Auth) authenticate(r *http.Request) authMethod {
	if len(a.apiKey) == 0 {
		return authNone
	}
	if key := r.Header.Get("X-Api-Key"); key != "" {
		if subtle.ConstantTimeCompare([]byte(key), a.apiKey) == 1 {
			return authAPIKey
		}
		return authNone
	}
	if c, err := r.Cookie(SessionCookieName); err == nil && a.verifyToken(c.Value) {
		return authSession
	}
	// A local address is only trusted under a host name a web page can't
	// own (see host.go): otherwise a DNS-rebinding page could use the
	// visitor's browser as its local address. Sessions and API keys don't
	// need this: a browser never sends Pilot's cookie to another domain.
	if !a.Settings().LoginRequiredEverywhere() && clientIsLocal(r) && a.hostTrusted(r) {
		return authLocal
	}
	return authNone
}

// untrustedHost reports whether r would have been let in as a local
// address but for its Host header, so the error can say what to do.
func (a *Auth) untrustedHost(r *http.Request) bool {
	return !a.Settings().LoginRequiredEverywhere() && clientIsLocal(r) && !a.hostTrusted(r)
}

func untrustedHostMessage(r *http.Request) string {
	return "Pilot was opened as \"" + hostName(r.Host) + "\", a name it doesn't recognise as this server, so it won't skip the login for it. " +
		"Use the server's IP address or local name, log in, or add this name to server.allowed_hosts (PILOT_SERVER_ALLOWED_HOSTS)."
}

// caller is who a request was let in as, for handlers that need to know.
type caller struct {
	method authMethod
	peer   string // the address that connected
}

type callerKey struct{}

func withCaller(r *http.Request, m authMethod) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), callerKey{}, caller{method: m, peer: peerAddr(r)}))
}

// peerAddr is the address that connected, without the port.
func peerAddr(r *http.Request) string {
	if ip, ok := parseIP(r.RemoteAddr); ok {
		return ip.String()
	}
	return r.RemoteAddr
}

// ErrWrongPassword and ErrTooManyAttempts are returned by
// ConfirmPassword.
var ErrWrongPassword = errors.New("the current password is wrong")

// ErrTooManyAttempts carries how long to wait.
type ErrTooManyAttempts struct{ Wait time.Duration }

func (e ErrTooManyAttempts) Error() string {
	return "Too many wrong passwords. Try again in " + waitText(e.Wait) + "."
}

func waitText(d time.Duration) string {
	if d <= time.Minute {
		return "a minute"
	}
	return fmt.Sprintf("%d minutes", int((d+time.Minute-1)/time.Minute))
}

// ConfirmPassword checks the current password before a change to the
// login itself, so a device that is merely on the network (let in
// without a login) can't replace the password and lock the owner out. A
// request made with the API key doesn't need it: the key is the stronger
// credential and is how a forgotten password gets reset. With no login
// set up there is nothing to confirm.
//
// ctx must come from a request that passed Middleware.
func (a *Auth) ConfirmPassword(ctx context.Context, password string) error {
	s := a.Settings()
	if !s.CredentialsConfigured() {
		return nil
	}
	c, _ := ctx.Value(callerKey{}).(caller)
	if c.method == authAPIKey {
		return nil
	}
	if d := a.limiter.wait(c.peer); d > 0 {
		return ErrTooManyAttempts{Wait: d}
	}
	if !CheckPassword(s.PasswordHash, password) {
		a.limiter.fail(c.peer)
		return ErrWrongPassword
	}
	a.limiter.ok(c.peer)
	return nil
}

// Middleware rejects requests that are not authenticated. Mount it on the
// route group that holds every protected route, not as a global
// path-prefix filter, so a route can't slip past it through path tricks.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := a.authenticate(r)
		r = withCaller(r, method)
		switch method {
		case authAPIKey:
			next.ServeHTTP(w, r)
		case authSession, authLocal:
			// Both are "ambient" credentials a browser supplies on its
			// own, so a page on another site must not be able to ride
			// them.
			if !crossSiteSafe(r) {
				writeAuthError(w, http.StatusForbidden, "Cross-site request rejected.")
				return
			}
			next.ServeHTTP(w, r)
		default:
			if a.untrustedHost(r) {
				writeAuthError(w, http.StatusUnauthorized, untrustedHostMessage(r))
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "Authentication required: log in or provide a valid X-Api-Key header.")
		}
	})
}

// crossSiteSafe is the CSRF check for cookie-authenticated requests.
// SameSite=Strict already stops browsers attaching the cookie to
// cross-site requests; this is a second layer that also covers
// cross-site WebSocket handshakes.
func crossSiteSafe(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		// Older browser without Fetch Metadata. Fall back to Origin
		// for anything that can change state.
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return true
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	default: // "same-site", "cross-site", or anything unexpected
		return false
	}
}

// StatusHandler tells the UI whether it is signed in and whether a login
// form makes sense. Unauthenticated.
//
//	GET /api/v1/auth/status
func (a *Auth) StatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := a.Settings()
		method := a.authenticate(r)
		if method == authLocal && !crossSiteSafe(r) {
			method = authNone
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		body := map[string]any{
			"authenticated":          method != authNone,
			"method":                 method.String(),
			"required":               s.Required,
			"credentials_configured": s.CredentialsConfigured(),
		}
		if method == authNone && a.untrustedHost(r) {
			// Tell the login screen why the UI didn't just open.
			body["untrusted_host"] = hostName(r.Host)
			body["detail"] = untrustedHostMessage(r)
		}
		_ = json.NewEncoder(w).Encode(body)
	}
}

// LoginHandler exchanges a username and password for a session cookie.
//
//	POST /api/v1/auth/login  {"username": "...", "password": "..."}
func (a *Auth) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !crossSiteSafe(r) {
			writeAuthError(w, http.StatusForbidden, "Cross-site request rejected.")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAuthError(w, http.StatusBadRequest, "Request body must be JSON with username and password fields.")
			return
		}
		s := a.Settings()
		if len(a.apiKey) == 0 || !s.CredentialsConfigured() {
			writeAuthError(w, http.StatusUnauthorized,
				"No login is set up yet. Open Pilot from your local network and set a username and password in Settings → App Settings.")
			return
		}
		peer := peerAddr(r)
		if d := a.limiter.wait(peer); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeAuthError(w, http.StatusTooManyRequests, ErrTooManyAttempts{Wait: d}.Error())
			return
		}
		// Always run the password hash so a wrong username takes as
		// long as a wrong password.
		passOK := CheckPassword(s.PasswordHash, body.Password)
		userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.Username)) == 1
		if !passOK || !userOK {
			a.limiter.fail(peer)
			writeAuthError(w, http.StatusUnauthorized, "Incorrect username or password.")
			return
		}
		a.limiter.ok(peer)
		token, err := a.issueToken()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "Could not create session.")
			return
		}
		// Secure is set only over HTTPS (isHTTPS): Pilot is usually reached
		// over plain HTTP on a LAN, where a Secure cookie would never be
		// sent back and login would silently fail. HttpOnly and
		// SameSite=Strict are always set.
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure is conditional on purpose
			Name:     SessionCookieName,
			Value:    token,
			Path:     "/",
			MaxAge:   int(a.ttl.Seconds()),
			HttpOnly: true,
			Secure:   isHTTPS(r),
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// LogoutHandler clears the session cookie. Sessions are stateless, so a
// copied cookie stays valid until it expires, the API key is rotated or
// the password changes.
//
//	POST /api/v1/auth/logout
func (a *Auth) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Secure is set only over HTTPS (isHTTPS): Pilot is usually reached
		// over plain HTTP on a LAN, where a Secure cookie would never be
		// sent back and login would silently fail. HttpOnly and
		// SameSite=Strict are always set.
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure is conditional on purpose
			Name:     SessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   isHTTPS(r),
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// issueToken returns "v1.<expiry-unix>.<nonce>.<sig>".
func (a *Auth) issueToken() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := sessionVersion + "." +
		strconv.FormatInt(a.now().Add(a.ttl).Unix(), 10) + "." +
		base64.RawURLEncoding.EncodeToString(nonce)
	return payload + "." + a.sign(payload), nil
}

func (a *Auth) verifyToken(token string) bool {
	if len(a.apiKey) == 0 {
		return false
	}
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return false
	}
	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 3 || parts[0] != sessionVersion {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return a.now().Unix() < exp
}

func (a *Auth) sign(payload string) string {
	a.mu.RLock()
	key := a.signKey
	a.mu.RUnlock()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeAuthError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"title":  http.StatusText(status),
		"detail": detail,
	})
}
