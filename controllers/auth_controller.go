package controllers

import (
	"net/http"
	"ticketing/config"
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
	// --- PERUBAHAN 1: Menangani pesan sukses/error dari URL (Solusi Masalah #2) ---
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"title":      "Login - Portal Ticketing",
			"query_next": r.URL.Query().Get("next"),
		}

		// Ambil pesan dari URL query parameter jika ada
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

		user, err := c.authService.Authenticate(username, password)
		if err != nil {
			utils.RenderTemplate(w, "login.html", map[string]interface{}{
				"error":            err.Error(),
				"entered_username": username,
				"query_next":       nextParam,
			})
			return
		}

		sess, _ := config.Store.Get(r, "session")
		sess.Values["user_id"] = user.ID
		sess.Values["username"] = user.Username
		sess.Save(r, w)

		// --- PERUBAHAN 2: Logika Redirect sesuai Role (Solusi Masalah #3) ---
		// Prioritaskan parameter 'next' jika ada
		if nextParam != "" {
			http.Redirect(w, r, config.Path(nextParam), http.StatusSeeOther)
			return
		}

		// Jika tidak ada 'next', cek role user
		// Staff & Admin masuk ke Dashboard Departemen, User biasa ke Dashboard User
		if user.IsStaff || user.IsSuperAdmin {
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
		// Kirim error map spesifik ke template jika gagal
		err := c.authService.RegisterUser(r.FormValue("username"), r.FormValue("email"), r.FormValue("password1"))
		if err != nil {
			utils.RenderTemplate(w, "register.html", map[string]interface{}{
				// Error "register" ini akan ditangkap oleh register.html yang baru
				"errors":   map[string]string{"register": err.Error()},
				"username": r.FormValue("username"),
				"email":    r.FormValue("email"),
			})
			return
		}

		// --- PERUBAHAN 3: Pesan yang lebih jelas (Solusi Masalah #2) ---
		http.Redirect(w, r, config.Path("/login")+"?success=Akun+berhasil+dibuat.+Cek+email+Anda+untuk+verifikasi+sebelum+login.", http.StatusSeeOther)
	}
}

func (c *AuthController) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := c.authService.VerifyEmail(token); err != nil {
		http.Redirect(w, r, config.Path("/login")+"?error=Verifikasi+gagal+atau+token+expired", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, config.Path("/login")+"?success=Email+terverifikasi.+Silakan+login", http.StatusSeeOther)
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := config.Store.Get(r, "session")
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

		err := c.authService.RequestPasswordReset(email)
		if err != nil {
			// Opsional: Handle error jika email tidak ditemukan (security best practice biasanya tidak memberitahu)
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

		if password != confirm {
			utils.RenderTemplate(w, "reset_password.html", map[string]interface{}{
				"error": "Password tidak cocok.",
				"token": token,
			})
			return
		}
		err := c.authService.ResetPassword(token, password)
		if err != nil {
			utils.RenderTemplate(w, "reset_password.html", map[string]interface{}{
				"error": "Gagal mereset password. Link mungkin sudah kadaluarsa.",
				"token": token,
			})
			return
		}

		http.Redirect(w, r, config.Path("/login")+"?success=Password+berhasil+diubah.+Silakan+login.", http.StatusSeeOther)
	}
}
