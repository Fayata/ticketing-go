package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/models"
)

type contextKey string

const (
	UserKey               contextKey = "user"
	AuthenticatedKey      contextKey = "authenticated"
	ActiveTicketsCountKey contextKey = "active_tickets_count"
)

var (
	activityMu       sync.Mutex
	lastUserActivity = make(map[uint]time.Time)
)

// TouchUserActivity records user activity timestamp and marks pending messages as delivered.
func TouchUserActivity(userID uint, isStaff bool, deptID *uint) {
	activityMu.Lock()
	lastTouch, exists := lastUserActivity[userID]
	now := time.Now()
	if exists && now.Sub(lastTouch) < 30*time.Second {
		activityMu.Unlock()
		return
	}
	lastUserActivity[userID] = now
	activityMu.Unlock()

	go func() {
		config.DB.Model(&models.User{}).Where("id = ?", userID).Update("last_active_at", now)

		// Auto-deliver any pending messages sent to this user
		config.DB.Model(&models.TicketReply{}).
			Where("ticket_id IN (SELECT id FROM tickets WHERE created_by_id = ?) AND user_id != ? AND is_delivered = ?", userID, userID, false).
			Update("is_delivered", true)

		if isStaff {
			q := config.DB.Model(&models.TicketReply{}).Where("user_id != ? AND is_delivered = ?", userID, false)
			if deptID != nil {
				q = q.Where("ticket_id IN (SELECT id FROM tickets WHERE assigned_to_id = ? OR department_id = ?)", userID, *deptID)
			} else {
				q = q.Where("ticket_id IN (SELECT id FROM tickets WHERE assigned_to_id = ?)", userID)
			}
			q.Update("is_delivered", true)
		}
	}()
}

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

		TouchUserActivity(user.ID, user.IsStaff || user.IsSuperAdmin, user.DepartmentID)

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

// LoggingMiddleware delegates to our structured logging.HTTPMiddleware.
func LoggingMiddleware(next http.Handler) http.Handler {
	return logging.HTTPMiddleware(next)
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

// EmailRequired memaksa user yang belum set email untuk pergi ke /set-email.
// Whitelist path: /set-email, /verify-email, /logout, /static/.
func EmailRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserKey).(*models.User)
		if !ok || user == nil {
			next.ServeHTTP(w, r)
			return
		}
		if user.NeedsEmailSetup() {
			// Izinkan akses ke path khusus agar tidak loop redirect
			p := r.URL.Path
			if p == config.Path("/set-email") ||
				p == config.Path("/verify-email") ||
				p == config.Path("/logout") ||
				len(p) >= len("/static/") && p[:len("/static/")] == "/static/" {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, config.Path("/set-email"), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

