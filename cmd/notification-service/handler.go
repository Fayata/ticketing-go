package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthCheckHandler returns the health status of the service
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"service": "notification-service",
		"status":  "ok",
	})
}

// rateLimiterData holds rate limiting information per IP
type rateLimiterData struct {
	count     int
	resetTime time.Time
}

var (
	clients = make(map[string]*rateLimiterData)
	mu      sync.Mutex
)

// RateLimiter adds rate limiting to notification endpoints: 30 req/min
func RateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr // Simplified, usually you'd want X-Real-IP or X-Forwarded-For

		mu.Lock()
		client, exists := clients[ip]
		if !exists || time.Now().After(client.resetTime) {
			clients[ip] = &rateLimiterData{
				count:     1,
				resetTime: time.Now().Add(1 * time.Minute),
			}
			mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		if client.count >= 30 {
			mu.Unlock()
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		client.count++
		mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
