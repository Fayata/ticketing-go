package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// SessionStore interface represents the session storage needed for CSRF
type SessionStore interface {
	Get(r *http.Request, key string) (string, error)
	Set(w http.ResponseWriter, r *http.Request, key, value string) error
}

const csrfTokenKey = "csrf_token"
const csrfHeaderKey = "X-CSRF-Token"
const csrfFormFieldKey = "csrf_token"

// GenerateCSRFToken generates a cryptographically random 32-byte CSRF token
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// CSRFMiddleware creates a new CSRF middleware
func CSRFMiddleware(store SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF for /api/ routes
			if strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// For safe methods, just ensure token exists in session
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				token, _ := store.Get(r, csrfTokenKey)
				if token == "" {
					token, _ = GenerateCSRFToken()
					_ = store.Set(w, r, csrfTokenKey, token)
				}
				next.ServeHTTP(w, r)
				return
			}

			// For unsafe methods (POST, PUT, DELETE, PATCH), validate token
			sessionToken, err := store.Get(r, csrfTokenKey)
			if err != nil || sessionToken == "" {
				http.Error(w, "Forbidden - Invalid CSRF Token", http.StatusForbidden)
				return
			}

			// Get token from header or form
			providedToken := r.Header.Get(csrfHeaderKey)
			if providedToken == "" {
				providedToken = r.FormValue(csrfFormFieldKey)
			}

			// Compare tokens safely
			if len(sessionToken) != len(providedToken) || subtle.ConstantTimeCompare([]byte(sessionToken), []byte(providedToken)) != 1 {
				http.Error(w, "Forbidden - Invalid CSRF Token", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
