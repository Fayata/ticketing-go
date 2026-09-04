package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"ticketing/config"
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
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(32 << 20); err == nil {
				if r.MultipartForm != nil && r.MultipartForm.Value != nil {
					for key, values := range r.MultipartForm.Value {
						for i, v := range values {
							r.MultipartForm.Value[key][i] = strings.TrimSpace(strings.ReplaceAll(v, "\x00", ""))
						}
					}
				}
			}
		}
		if err := r.ParseForm(); err == nil {
			for key, values := range r.PostForm {
				for i, v := range values {
					r.PostForm[key][i] = strings.TrimSpace(strings.ReplaceAll(v, "\x00", ""))
				}
			}
			for key, values := range r.Form {
				for i, v := range values {
					r.Form[key][i] = strings.TrimSpace(strings.ReplaceAll(v, "\x00", ""))
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

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			if err := r.ParseForm(); err != nil {
				http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Format data tidak valid"), http.StatusSeeOther)
				return
			}
		}

		if r.MultipartForm != nil {
			var attCount int
			for _, fh := range r.MultipartForm.File["attachments"] {
				if fh != nil && strings.TrimSpace(fh.Filename) != "" {
					attCount++
				}
			}
			if attCount > 5 {
				http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Maksimal 5 file gambar yang dapat dilampirkan"), http.StatusSeeOther)
				return
			}
		}

		title := strings.TrimSpace(r.FormValue("title"))
		description := strings.TrimSpace(r.FormValue("description"))
		email := strings.TrimSpace(r.FormValue("reply_to_email"))
		priority := strings.TrimSpace(r.FormValue("priority"))

		if len(title) < 3 || len(title) > 200 {
			http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Judul tiket harus antara 3 hingga 200 karakter"), http.StatusSeeOther)
			return
		}

		if len(description) < 10 || len(description) > 10000 {
			http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Deskripsi masalah minimal harus 10 karakter"), http.StatusSeeOther)
			return
		}

		emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
		if !emailRegex.MatchString(email) {
			http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Format alamat email tidak valid"), http.StatusSeeOther)
			return
		}

		if priority != "" && priority != "LOW" && priority != "MEDIUM" && priority != "HIGH" {
			http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Prioritas tidak valid"), http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}
