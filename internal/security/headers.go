package security

import (
	"net/http"
)

// SecurityHeadersMiddleware adds security headers to the response
func SecurityHeadersMiddleware(debug bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

			if !debug {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
				w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'")
			} else {
				w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'")
			}

			next.ServeHTTP(w, r)
		})
	}
}
