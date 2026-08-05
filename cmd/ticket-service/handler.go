package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// HealthCheckHandler returns the health status of the service
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"service": "ticket-service",
		"status":  "ok",
	})
}

// MethodValidator ensures only the allowed HTTP method is used
func MethodValidator(allowedMethod string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != allowedMethod {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// InputSanitizer sanitizes form inputs
func InputSanitizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			for key, values := range r.PostForm {
				for i, v := range values {
					// Trim whitespace and remove null bytes
					sanitized := strings.TrimSpace(strings.ReplaceAll(v, "\x00", ""))
					// Optional: Use security.SanitizeInput if available in project
					// sanitized = security.SanitizeInput(sanitized)
					r.PostForm[key][i] = sanitized
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ValidateTicketInput validates ticket creation input
func ValidateTicketInput(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		title := r.PostFormValue("title")
		description := r.PostFormValue("description")
		email := r.PostFormValue("reply_to_email")
		priority := r.PostFormValue("priority")

		if len(title) < 3 || len(title) > 200 {
			http.Error(w, "Title must be between 3 and 200 characters", http.StatusBadRequest)
			return
		}

		if len(description) < 10 || len(description) > 10000 {
			http.Error(w, "Description must be between 10 and 10000 characters", http.StatusBadRequest)
			return
		}

		emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
		if !emailRegex.MatchString(email) {
			http.Error(w, "Invalid email format", http.StatusBadRequest)
			return
		}

		if priority != "LOW" && priority != "MEDIUM" && priority != "HIGH" {
			http.Error(w, "Priority must be LOW, MEDIUM, or HIGH", http.StatusBadRequest)
			return
		}

		// Rewrite the body or form to continue
		next.ServeHTTP(w, r)
	})
}
