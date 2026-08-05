package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
)

// HealthCheckHandler provides a simple health check endpoint for the admin service.
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "admin-service",
	})
}

// AuditLogWrapper wraps an http.HandlerFunc to log administrative operations.
func AuditLogWrapper(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[AUDIT] Action: %s, Method: %s, Path: %s, IP: %s", action, r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	}
}

// ValidateUserCreation validates user creation input (username, email, password).
func ValidateUserCreation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			username := r.FormValue("username")
			email := r.FormValue("email")
			password := r.FormValue("password")

			if len(username) < 3 {
				http.Error(w, "Username must be at least 3 characters", http.StatusBadRequest)
				return
			}

			emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
			if !emailRegex.MatchString(email) {
				http.Error(w, "Invalid email format", http.StatusBadRequest)
				return
			}

			if len(password) < 6 {
				http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}
