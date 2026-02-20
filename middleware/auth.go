package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"ticketing/config"
	"ticketing/models"
)

type contextKey string

const (
	UserKey               contextKey = "user"
	AuthenticatedKey      contextKey = "authenticated"
	ActiveTicketsCountKey contextKey = "active_tickets_count"
)

// AuthRequired middleware untuk memastikan user sudah login
func AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := config.Store.Get(r, "session")
		if err != nil {
			http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
			return
		}

		userID := sess.Values["user_id"]
		if userID == nil {
			http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
			return
		}

		// Load user dari database
		var user models.User
		if err := config.DB.Preload("Groups").Preload("Department").First(&user, userID).Error; err != nil {
			sess.Options.MaxAge = -1
			sess.Save(r, w)
			http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
			return
		}

		// Set user ke context
		ctx := context.WithValue(r.Context(), UserKey, &user)
		ctx = context.WithValue(ctx, AuthenticatedKey, true)

		// Count active tickets
		var activeCount int64
		config.DB.Model(&models.Ticket{}).
			Where("created_by_id = ? AND status != ?", user.ID, models.StatusClosed).
			Count(&activeCount)
		ctx = context.WithValue(ctx, ActiveTicketsCountKey, activeCount)

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// PortalUserRequired middleware untuk memastikan user memiliki akses portal
func PortalUserRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(UserKey).(*models.User)

		if !user.HasPortalAccess() {
			http.Error(w, "Akses ini khusus untuk akun pengguna portal.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// GuestOnly middleware untuk halaman yang hanya boleh diakses user yang belum login
func GuestOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := config.Store.Get(r, "session")
		if err == nil {
			userID := sess.Values["user_id"]
			if userID != nil {
				http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
				return
			}
		}

		next.ServeHTTP(w, r)
	}
}

// SetUserLocals middleware untuk set user info ke semua template
func SetUserLocals(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := config.Store.Get(r, "session")
		if err == nil {
			userID := sess.Values["user_id"]
			if userID != nil {
				var user models.User
				if err := config.DB.Preload("Groups").First(&user, userID).Error; err == nil {
					ctx := context.WithValue(r.Context(), UserKey, &user)
					ctx = context.WithValue(ctx, AuthenticatedKey, true)

					// Count active tickets
					var activeCount int64
					config.DB.Model(&models.Ticket{}).
						Where("created_by_id = ? AND status != ?", user.ID, models.StatusClosed).
						Count(&activeCount)
					ctx = context.WithValue(ctx, ActiveTicketsCountKey, activeCount)

					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		ctx := context.WithValue(r.Context(), AuthenticatedKey, false)
		ctx = context.WithValue(ctx, ActiveTicketsCountKey, 0)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs all HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.RequestURI, time.Since(start))
		
	})
	
}

func DepartmentRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(UserKey).(*models.User)

		if !user.IsStaff {
			http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func SuperAdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(UserKey).(*models.User)

		if !user.IsSuperAdmin {
			if user.IsStaff {
				http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
			} else {
				http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
			}
			return
		}

		next.ServeHTTP(w, r)
	}
}
