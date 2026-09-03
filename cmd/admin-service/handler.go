package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"ticketing/config"
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
			username := strings.TrimSpace(r.FormValue("username"))
			email := strings.TrimSpace(r.FormValue("email"))
			password := r.FormValue("password")

			if len(username) < 3 {
				http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Username+minimal+3+karakter", http.StatusSeeOther)
				return
			}

			emailRegex := regexp.MustCompile(`(?i)^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
			if !emailRegex.MatchString(email) {
				http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Format+email+tidak+valid", http.StatusSeeOther)
				return
			}

			if len(password) < 6 {
				http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Password+minimal+6+karakter", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}
