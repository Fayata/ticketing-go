package main

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"ticketing/config"
	"ticketing/controllers"
	"ticketing/internal/security"
)

type AuthHandler struct {
	ctrl        *controllers.AuthController
	authService *AuthServiceWrapper
}

func NewAuthHandler(authService *AuthServiceWrapper) *AuthHandler {
	return &AuthHandler{
		ctrl:        controllers.NewAuthController(authService.AuthService),
		authService: authService,
	}
}

var (
	loginAttempts = make(map[string]loginAttempt)
	mu            sync.Mutex
)

type loginAttempt struct {
	count     int
	timestamp time.Time
}

func getIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	return ip
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ip := getIP(r)
		mu.Lock()
		attempt, exists := loginAttempts[ip]
		if exists && attempt.count >= 5 && time.Since(attempt.timestamp) < 15*time.Minute {
			mu.Unlock()
			security.LogAudit(security.AuditEvent{Action: "LOGIN_LOCKED", Details: "IP locked out due to multiple failed attempts: " + ip, Timestamp: time.Now()})
			http.Error(w, "Terlalu banyak percobaan login. Coba lagi dalam 15 menit.", http.StatusTooManyRequests)
			return
		}
		mu.Unlock()

		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")

		if security.ValidateUsername(username) != nil {
			security.LogAudit(security.AuditEvent{Action: "LOGIN_FAILED", Details: "Invalid username format: " + username, Timestamp: time.Now()})
			h.recordFailedLogin(ip)
			http.Redirect(w, r, config.Path("/login")+"?error=Format+username+tidak+valid", http.StatusSeeOther)
			return
		}

		user, err := h.authService.Authenticate(username, password)
		if err != nil {
			security.LogAudit(security.AuditEvent{Action: "LOGIN_FAILED", Details: "Failed login for username: " + username, Timestamp: time.Now()})
			h.recordFailedLogin(ip)
			h.ctrl.Login(w, r)
			return
		}

		// Success - reset attempts
		mu.Lock()
		delete(loginAttempts, ip)
		mu.Unlock()

		// Session Fixation Protection: regenerate session
		sess, _ := config.Store.Get(r, "session")
		sess.Options.MaxAge = -1
		sess.Save(r, w) // Delete old session

		// Remove cookie from request so Store.Get generates a new one
		r.Header.Del("Cookie")

		newSess, _ := config.Store.Get(r, "session")
		newSess.Values["user_id"] = user.ID
		newSess.Values["username"] = user.Username
		newSess.Save(r, w)

		security.LogAudit(security.AuditEvent{Action: "LOGIN_SUCCESS", Details: "User logged in: " + username, Timestamp: time.Now(), UserID: user.ID})

		// Call original to handle redirects, but we already authenticated and set the session!
		// Wait, if we call h.ctrl.Login now, it will re-authenticate and re-set session.
		// That's fine, but let's just let it run. Wait, no, we removed the cookie so the new session is set.
		// Wait, if we call h.ctrl.Login, it will call Authenticate again.
		// Let's just do the redirect ourselves here instead of calling the controller, to be safe.
		if user.IsSuperAdmin {
			http.Redirect(w, r, config.Path("/admin/dashboard"), http.StatusSeeOther)
		} else if user.IsStaff {
			http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
		} else {
			http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
		}
		return
	}

	h.ctrl.Login(w, r)
}

func (h *AuthHandler) recordFailedLogin(ip string) {
	mu.Lock()
	defer mu.Unlock()
	attempt := loginAttempts[ip]
	attempt.count++
	attempt.timestamp = time.Now()
	loginAttempts[ip] = attempt
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		email := r.FormValue("email")
		
		if security.ValidateUsername(username) != nil || security.ValidateEmail(email) != nil {
			security.LogAudit(security.AuditEvent{Action: "REGISTER_FAILED", Details: "Invalid input format", Timestamp: time.Now()})
			http.Redirect(w, r, config.Path("/register")+"?error=Input+tidak+valid", http.StatusSeeOther)
			return
		}

		security.LogAudit(security.AuditEvent{Action: "REGISTER_ATTEMPT", Details: "Attempt to register user: " + username, Timestamp: time.Now()})
	}

	h.ctrl.Register(w, r)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := config.Store.Get(r, "session")
	username, _ := sess.Values["username"].(string)
	if username != "" {
		security.LogAudit(security.AuditEvent{Action: "LOGOUT", Details: "User logged out: " + username, Timestamp: time.Now()})
	}
	h.ctrl.Logout(w, r)
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	h.ctrl.VerifyEmail(w, r)
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	h.ctrl.ForgotPassword(w, r)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		security.LogAudit(security.AuditEvent{Action: "PASSWORD_RESET", Details: "Password reset attempted", Timestamp: time.Now()})
	}
	h.ctrl.ResetPassword(w, r)
}
