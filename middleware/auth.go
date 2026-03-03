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

// AuthRequired memastikan user sudah login; load user ke context, redirect ke login jika belum.
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

		var user models.User
		if err := config.DB.Preload("Groups").Preload("Department").First(&user, userID).Error; err != nil {
			sess.Options.MaxAge = -1
			sess.Save(r, w)
			http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserKey, &user)
		ctx = context.WithValue(ctx, AuthenticatedKey, true)

		var activeCount int64
		config.DB.Model(&models.Ticket{}).
			Where("created_by_id = ? AND status != ?", user.ID, models.StatusClosed).
			Count(&activeCount)
		ctx = context.WithValue(ctx, ActiveTicketsCountKey, activeCount)

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// PortalUserRequired membatasi akses ke portal user; staff/admin di-redirect ke dashboard masing-masing.
func PortalUserRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(UserKey).(*models.User)

		if !user.HasPortalAccess() {
			http.Error(w, "Akses ini khusus untuk akun pengguna portal.", http.StatusForbidden)
			return
		}
		if user.IsSuperAdmin {
			http.Redirect(w, r, config.Path("/admin/dashboard"), http.StatusSeeOther)
			return
		}
		if user.IsStaff {
			http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// GuestOnly untuk halaman login/register; jika sudah login redirect ke dashboard sesuai role.
func GuestOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := config.Store.Get(r, "session")
		if err == nil {
			userID := sess.Values["user_id"]
			if userID != nil {
				var user models.User
				if err := config.DB.Select("is_staff", "is_super_admin").First(&user, userID).Error; err == nil {
					if user.IsSuperAdmin {
						http.Redirect(w, r, config.Path("/admin/dashboard"), http.StatusSeeOther)
						return
					}
					if user.IsStaff {
						http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
						return
					}
				}
				http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
				return
			}
		}

		next.ServeHTTP(w, r)
	}
}

// Isi context dengan user (buat dipakai di template).
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

// LoggingMiddleware mencatat request POST/PUT/DELETE/PATCH ke log (GET tidak dicatat).
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		m := r.Method
		if m == http.MethodPost || m == http.MethodPut || m == http.MethodDelete || m == http.MethodPatch {
			log.Printf("[%s] %s %s", m, r.RequestURI, time.Since(start))
		}
	})
}

// DepartmentRequired membatasi akses ke halaman staff; user biasa di-redirect ke dashboard.
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

// SuperAdminRequired membatasi akses ke halaman admin; hanya super admin yang boleh.
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

// StaffOrSuperAdminRequired untuk akses KB admin; cukup staff atau super admin.
func StaffOrSuperAdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(UserKey).(*models.User)
		if !user.IsStaff && !user.IsSuperAdmin {
			http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
