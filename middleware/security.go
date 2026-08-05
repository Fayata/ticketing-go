package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ticketing/config"
)

const csrfContextKey contextKey = "csrf_token"

// SecurityHeaders applies security-related HTTP headers to the response
func SecurityHeaders(next http.Handler, debug bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// [Security] CSP: tighter in production (no unsafe-eval)
		if debug {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com https://cdn.jsdelivr.net; img-src 'self' data: blob:; connect-src 'self'")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com https://cdn.jsdelivr.net; img-src 'self' data: blob:; connect-src 'self'")
			// [Security] HSTS: enforce HTTPS for 1 year in production
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// [Security] Prevent caching of sensitive pages
		if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			w.Header().Set("Pragma", "no-cache")
		}

		next.ServeHTTP(w, r)
	})
}

// Generate random CSRF token
func generateCSRFToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Printf("[Security][CSRF] Error generating token: %v", err)
		return ""
	}
	return hex.EncodeToString(bytes)
}

// CSRFMiddleware protects against Cross-Site Request Forgery
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF check for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		session, err := config.Store.Get(r, "session")
		if err != nil {
			log.Printf("[Security][CSRF] Session error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Retrieve or generate CSRF token
		var token string
		if t, ok := session.Values["csrf_token"].(string); ok && t != "" {
			token = t
		} else {
			token = generateCSRFToken()
			session.Values["csrf_token"] = token
			if err := session.Save(r, w); err != nil {
				log.Printf("[Security][CSRF] Failed to save session: %v", err)
			}
		}

		// Add token to context
		ctx := context.WithValue(r.Context(), csrfContextKey, token)
		r = r.WithContext(ctx)

		// Validate on state-changing methods
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
			requestToken := r.Header.Get("X-CSRF-Token")
			if requestToken == "" {
				requestToken = r.FormValue("csrf_token")
			}

			if subtle.ConstantTimeCompare([]byte(requestToken), []byte(token)) != 1 {
				log.Printf("[Security][CSRF] Token mismatch for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
				http.Error(w, "403 Forbidden - CSRF token invalid", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// GetCSRFToken retrieves the CSRF token from the request context
func GetCSRFToken(r *http.Request) string {
	if token, ok := r.Context().Value(csrfContextKey).(string); ok {
		return token
	}
	return ""
}

type RateLimiter struct {
	maxRequests    int
	windowDuration time.Duration
	visitors       map[string][]time.Time
	mu             sync.Mutex
}

func NewRateLimiter(maxRequests int, windowDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		maxRequests:    maxRequests,
		windowDuration: windowDuration,
		visitors:       make(map[string][]time.Time),
	}
	// [Security] Auto-cleanup expired entries every 5 minutes to prevent memory leak
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			rl.mu.Lock()
			now := time.Now()
			for ip, times := range rl.visitors {
				var valid []time.Time
				for _, t := range times {
					if now.Sub(t) <= rl.windowDuration {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.visitors, ip)
				} else {
					rl.visitors[ip] = valid
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

// getClientIP extracts the real client IP, supporting reverse proxies.
func getClientIP(r *http.Request) string {
	// Check X-Real-IP first
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (rl *RateLimiter) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		rl.mu.Lock()
		now := time.Now()

		var validTimes []time.Time
		if times, exists := rl.visitors[ip]; exists {
			for _, t := range times {
				if now.Sub(t) <= rl.windowDuration {
					validTimes = append(validTimes, t)
				}
			}
		}

		if len(validTimes) >= rl.maxRequests {
			rl.visitors[ip] = validTimes
			rl.mu.Unlock()

			retryAfter := rl.windowDuration.Seconds()
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter))
			log.Printf("[Security][RateLimit] IP %s exceeded %d requests in %v on %s", ip, rl.maxRequests, rl.windowDuration, r.URL.Path)
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		validTimes = append(validTimes, now)
		rl.visitors[ip] = validTimes
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	}
}

// LimitHandler wraps http.Handler (for use in middleware chains).
func (rl *RateLimiter) LimitHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		rl.mu.Lock()
		now := time.Now()

		var validTimes []time.Time
		if times, exists := rl.visitors[ip]; exists {
			for _, t := range times {
				if now.Sub(t) <= rl.windowDuration {
					validTimes = append(validTimes, t)
				}
			}
		}

		if len(validTimes) >= rl.maxRequests {
			rl.visitors[ip] = validTimes
			rl.mu.Unlock()

			retryAfter := rl.windowDuration.Seconds()
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter))
			log.Printf("[Security][RateLimit] IP %s exceeded %d requests in %v on %s", ip, rl.maxRequests, rl.windowDuration, r.URL.Path)
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		validTimes = append(validTimes, now)
		rl.visitors[ip] = validTimes
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
