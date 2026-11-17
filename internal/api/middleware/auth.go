package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SessionCookieName is the cookie that carries a browser session.
const SessionCookieName = "pilot_session"

// DefaultSessionTTL is how long a browser session stays valid after login.
const DefaultSessionTTL = 30 * 24 * time.Hour

// sessionVersion prefixes every token so the format can change later
// without old tokens being misparsed.
const sessionVersion = "v1"

// Auth authenticates API requests. A request is accepted when it carries
// either:
//
//   - a valid X-Api-Key header (scripts, Pulse, other services), or
//   - a valid session cookie issued by Login (the browser UI).
//
// Sec-Fetch-Site is deliberately NOT a credential: any non-browser client
// can set it to "same-origin". It is only used as a CSRF check on
// cookie-authenticated requests, where the browser guarantees its value.
//
// Sessions are stateless HMAC-signed tokens. The signing key is derived
// from the API key, so rotating the API key invalidates every session.
type Auth struct {
	apiKey  []byte
	signKey []byte
	ttl     time.Duration
	now     func() time.Time
}

// NewAuth builds an Auth for the given API key. An empty key rejects
// every request (fail closed).
func NewAuth(apiKey string) *Auth {
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte("pilot-session-" + sessionVersion))
	return &Auth{
		apiKey:  []byte(apiKey),
		signKey: mac.Sum(nil),
		ttl:     DefaultSessionTTL,
		now:     time.Now,
	}
}

type authMethod int

const (
	authNone authMethod = iota
	authAPIKey
	authSession
)

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
	return authNone
}

// Middleware rejects requests that are not authenticated. Mount it on the
// route group that holds every protected route, not as a global
// path-prefix filter, so a route can't slip past it through path tricks.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch a.authenticate(r) {
		case authAPIKey:
			next.ServeHTTP(w, r)
		case authSession:
			if !crossSiteSafe(r) {
				writeAuthError(w, http.StatusForbidden, "Cross-site request rejected.")
				return
			}
			next.ServeHTTP(w, r)
		default:
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

// LoginHandler exchanges the API key for a session cookie.
//
//	POST /api/v1/auth/login  {"api_key": "..."}
func (a *Auth) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !crossSiteSafe(r) {
			writeAuthError(w, http.StatusForbidden, "Cross-site request rejected.")
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAuthError(w, http.StatusBadRequest, "Request body must be JSON with an api_key field.")
			return
		}
		if len(a.apiKey) == 0 || subtle.ConstantTimeCompare([]byte(body.APIKey), a.apiKey) != 1 {
			writeAuthError(w, http.StatusUnauthorized, "Invalid API key.")
			return
		}
		token, err := a.issueToken()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "Could not create session.")
			return
		}
		http.SetCookie(w, &http.Cookie{
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
// copied cookie stays valid until it expires or the API key is rotated.
//
//	POST /api/v1/auth/logout
func (a *Auth) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
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
	mac := hmac.New(sha256.New, a.signKey)
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
