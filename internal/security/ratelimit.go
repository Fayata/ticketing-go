package security

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter struct for IP-based rate limiting
type RateLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*visitor
	maxRequests int
	window      time.Duration
}

type visitor struct {
	count     int
	expiresAt time.Time
}

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors:    make(map[string]*visitor),
		maxRequests: maxRequests,
		window:      window,
	}

	// Background cleanup
	go func() {
		for {
			time.Sleep(window / 2)
			rl.cleanup()
		}
	}()

	return rl
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for ip, v := range rl.visitors {
		if now.After(v.expiresAt) {
			delete(rl.visitors, ip)
		}
	}
}

// getIP gets the IP address from the request
func getIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ips := strings.Split(ip, ",")
		return strings.TrimSpace(ips[0])
	}
	ip = r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}
	// Fallback to RemoteAddr
	addr := r.RemoteAddr
	idx := strings.LastIndex(addr, ":")
	if idx != -1 {
		return addr[:idx]
	}
	return addr
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		now := time.Now()

		if !exists || now.After(v.expiresAt) {
			rl.visitors[ip] = &visitor{
				count:     1,
				expiresAt: now.Add(rl.window),
			}
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		v.count++
		if v.count > rl.maxRequests {
			retryAfter := v.expiresAt.Sub(now).Seconds()
			rl.mu.Unlock()
			
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter)+1))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
