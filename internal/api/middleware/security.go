package middleware

import (
	"net/http"
)

// SecurityHeaders sets defensive HTTP response headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		// style-src must include fonts.googleapis.com so the @import
		// stylesheet (which @font-faces Inter / JetBrains Mono) loads.
		// font-src must include fonts.gstatic.com for the actual woff2
		// files. Without both, Inter fails silently and the nav falls
		// back to system-ui — visibly different on Linux/Windows.
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data: https://image.tmdb.org; font-src 'self' https://fonts.gstatic.com; connect-src 'self' blob:; worker-src 'self' blob:")
		next.ServeHTTP(w, r)
	})
}

// MaxRequestBodySize limits request bodies to maxBytes.
func MaxRequestBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
