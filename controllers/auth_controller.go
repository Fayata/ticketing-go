package controllers

import (
	"net/http"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/services"
	"ticketing/utils"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController(authService *services.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"title":      "Login - Portal Ticketing",
			"query_next": r.URL.Query().Get("next"),
		}

		// pesan sukses/error dari query string (misal setelah verifikasi email)
		if successMsg := r.URL.Query().Get("success"); successMsg != "" {
			data["success"] = successMsg
		}
		if errorMsg := r.URL.Query().Get("error"); errorMsg != "" {
			data["error"] = errorMsg
		}

		utils.RenderTemplate(w, "login.html", data)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")
		nextParam := r.FormValue("next")
		clientIP := logging.GetClientIP(r)

		user, err := c.authService.Authenticate(username, password)
		if err != nil {
			logging.AuthLogin.Warn("Login failed",
				"username", username,
				"ip", clientIP,
				"error", err.Error(),
			)
			utils.RenderTemplate(w, "login.html", map[string]interface{}{
				"error":            err.Error(),
				"entered_username": username,
				"query_next":       nextParam,
			})
			return
		}

		now := time.Now()
		config.DB.Model(user).Update("last_login", now)
		user.LastLogin = &now

		sess, _ := config.Store.Get(r, "session")
		sess.Values["user_id"] = user.ID
		sess.Values["username"] = user.Username
		sess.Save(r, w)

		role := "user"
		if user.IsSuperAdmin {
			role = "superadmin"
		} else if user.IsStaff {
			role = "staff"
		}

		logging.AuthLogin.Info("Login successful",
			"user_id", user.ID,
			"username", user.Username,
			"role", role,
			"ip", clientIP,
		)

		if nextParam != "" {
			// [Security] Validasi open redirect — hanya izinkan relative path internal
			if strings.HasPrefix(nextParam, "/") && !strings.HasPrefix(nextParam, "//") && !strings.Contains(nextParam, ":") {
				logging.AuthSecurity.Info("Valid next redirect accepted", "next", nextParam, "username", user.Username)
				http.Redirect(w, r, config.Path(nextParam), http.StatusSeeOther)
				return
			}
			logging.AuthSecurity.Warn("BLOCKED open redirect attempt", "blocked_url", nextParam, "username", user.Username, "ip", clientIP)
		}
		// setelah login: admin -> dashboard admin, staff -> departemen, user -> dashboard
		if user.IsSuperAdmin {
			http.Redirect(w, r, config.Path("/admin/dashboard"), http.StatusSeeOther)
		} else if user.IsStaff {
			http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
		} else {
			http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
		}
	}
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		utils.RenderTemplate(w, "register.html", map[string]interface{}{
			"title": "Registrasi - Portal Ticketing",
		})
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		email := r.FormValue("email")
		clientIP := logging.GetClientIP(r)

		err := c.authService.RegisterUser(username, email, r.FormValue("password1"))
		if err != nil {
			logging.AuthRegister.Warn("User registration failed",
				"username", username,
				"email", email,
				"error", err.Error(),
				"ip", clientIP,
			)
			utils.RenderTemplate(w, "register.html", map[string]interface{}{
				"errors":   map[string]string{"register": err.Error()},
				"username": username,
				"email":    email,
			})
			return
		}

		logging.AuthRegister.Info("User registered successfully",
			"username", username,
			"email", email,
			"ip", clientIP,
		)

		http.Redirect(w, r, config.Path("/login")+"?success=Akun+berhasil+dibuat.+Cek+email+Anda+untuk+verifikasi+sebelum+login.", http.StatusSeeOther)
	}
}

func (c *AuthController) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	clientIP := logging.GetClientIP(r)

	if err := c.authService.VerifyEmail(token); err != nil {
		logging.AuthEmailVerify.Warn("Email verification failed",
			"token", token,
			"ip", clientIP,
			"error", err.Error(),
		)
		http.Redirect(w, r, config.Path("/login")+"?error=Verifikasi+gagal+atau+token+expired", http.StatusSeeOther)
		return
	}

	logging.AuthEmailVerify.Info("Email successfully verified",
		"token", token,
		"ip", clientIP,
	)
	http.Redirect(w, r, config.Path("/login")+"?success=Email+terverifikasi.+Silakan+login", http.StatusSeeOther)
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := config.Store.Get(r, "session")
	uid, _ := sess.Values["user_id"]
	uname, _ := sess.Values["username"]

	logging.AuthLogin.Info("User logged out",
		"user_id", uid,
		"username", uname,
		"ip", logging.GetClientIP(r),
	)

	sess.Options.MaxAge = -1
	sess.Save(r, w)
	http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
}

func (c *AuthController) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		utils.RenderTemplate(w, "forgot_password.html", map[string]interface{}{
			"title": "Lupa Password - Portal Ticketing",
		})
		return
	}

	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		clientIP := logging.GetClientIP(r)

		err := c.authService.RequestPasswordReset(email)
		if err != nil {
			logging.AuthPasswordReset.Warn("Password reset request error",
				"email", email,
				"ip", clientIP,
				"error", err.Error(),
			)
		} else {
			logging.AuthPasswordReset.Info("Password reset instructions sent",
				"email", email,
				"ip", clientIP,
			)
		}

		utils.RenderTemplate(w, "forgot_password.html", map[string]interface{}{
			"success": "Instruksi reset password telah dikirim ke email Anda.",
		})
	}
}

func (c *AuthController) ResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == http.MethodPost {
		token = r.FormValue("token")
	}

	if token == "" {
		http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		utils.RenderTemplate(w, "reset_password.html", map[string]interface{}{
			"title": "Buat Password Baru",
			"token": token,
		})
		return
	}

	if r.Method == http.MethodPost {
		password := r.FormValue("password")
		confirm := r.FormValue("confirm_password")
		clientIP := logging.GetClientIP(r)

		if password != confirm {
			utils.RenderTemplate(w, "reset_password.html", map[string]interface{}{
				"error": "Password tidak cocok.",
				"token": token,
			})
			return
		}
		err := c.authService.ResetPassword(token, password)
		if err != nil {
			logging.AuthPasswordReset.Warn("Reset password failed",
				"token", token,
				"ip", clientIP,
				"error", err.Error(),
			)
			utils.RenderTemplate(w, "reset_password.html", map[string]interface{}{
				"error": "Gagal mereset password. Link mungkin sudah kadaluarsa.",
				"token": token,
			})
			return
		}

		logging.AuthPasswordReset.Info("Password reset successfully completed",
			"token", token,
			"ip", clientIP,
		)

		http.Redirect(w, r, config.Path("/login")+"?success=Password+berhasil+diubah.+Silakan+login.", http.StatusSeeOther)
	}
}
