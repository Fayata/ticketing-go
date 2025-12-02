package handlers

import (
	"log"
	"net/http"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// ShowLogin menampilkan halaman login
func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := map[string]interface{}{
		"title": "Login - Portal Ticketing",
	}

	if next := r.URL.Query().Get("next"); next != "" {
		data["query_next"] = next
	}

	if r.URL.Query().Get("registered") == "true" {
		data["success"] = "Akun berhasil dibuat. Silakan login untuk melanjutkan."
	}

	// CHANGED: Use "login.html" instead of "tickets/login.html"
	RenderTemplate(w, "login.html", data)
}

// Login proses login user
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	rememberMe := r.FormValue("remember_me")
	nextParam := r.FormValue("next")

	log.Printf("Login attempt for user: %s", username)

	// Cari user berdasarkan username atau email
	var user models.User
	if err := config.DB.Preload("Groups").
		Where("username = ? OR email = ?", username, username).
		First(&user).Error; err != nil {
		log.Printf("User not found: %s", username)
		// CHANGED: Use "login.html"
		RenderTemplate(w, "login.html", map[string]interface{}{
			"error":            "Username atau password salah. Silakan coba lagi.",
			"title":            "Login - Portal Ticketing",
			"query_next":       nextParam,
			"entered_username": username,
		})
		return
	}

	// Cek password
	if !utils.CheckPasswordHash(password, user.Password) {
		log.Printf("Invalid password for user: %s", username)
		// CHANGED: Use "login.html"
		RenderTemplate(w, "login.html", map[string]interface{}{
			"error":            "Username atau password salah. Silakan coba lagi.",
			"title":            "Login - Portal Ticketing",
			"query_next":       nextParam,
			"entered_username": username,
		})
		return
	}

	// Cek akses portal
	if !user.HasPortalAccess() {
		log.Printf("User %s doesn't have portal access", username)
		// CHANGED: Use "login.html"
		RenderTemplate(w, "login.html", map[string]interface{}{
			"error":            "Akun ini tidak memiliki akses ke dashboard pengguna.",
			"title":            "Login - Portal Ticketing",
			"query_next":       nextParam,
			"entered_username": username,
		})
		return
	}

	// Update last login
	now := time.Now()
	user.LastLogin = &now
	config.DB.Save(&user)

	// Create session
	sess, err := config.Store.Get(r, "session")
	if err != nil {
		log.Printf("Session error: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	sess.Values["user_id"] = user.ID
	sess.Values["username"] = user.Username

	// Set session expiry
	if rememberMe == "" {
		sess.Options.MaxAge = 0 // Session expires when browser closes
	} else {
		sess.Options.MaxAge = 14 * 24 * 3600 // 2 weeks
	}

	if err := sess.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	log.Printf("Login successful for user: %s", username)

	// Check for next parameter
	next := nextParam
	if next == "" {
		next = r.URL.Query().Get("next")
	}

	if next != "" {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ShowRegister menampilkan halaman registrasi
func (h *AuthHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// CHANGED: Use "register.html"
	RenderTemplate(w, "register.html", map[string]interface{}{
		"title": "Registrasi - Portal Ticketing",
	})
}

// Register proses registrasi user baru
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	username := r.FormValue("username")
	email := r.FormValue("email")
	password1 := r.FormValue("password1")
	password2 := r.FormValue("password2")

	// Validasi
	errors := make(map[string]string)

	if username == "" {
		errors["username"] = "Username wajib diisi"
	}
	if email == "" {
		errors["email"] = "Email wajib diisi"
	}
	if password1 == "" {
		errors["password1"] = "Password wajib diisi"
	}
	if password1 != password2 {
		errors["password2"] = "Password tidak cocok"
	}

	// Cek username exists
	var existingUser models.User
	if err := config.DB.Where("username = ?", username).First(&existingUser).Error; err == nil {
		errors["username"] = "Username sudah digunakan"
	}

	// Cek email exists
	if err := config.DB.Where("email = ?", email).First(&existingUser).Error; err == nil {
		errors["email"] = "Email sudah terdaftar"
	}

	if len(errors) > 0 {
		// CHANGED: Use "register.html"
		RenderTemplate(w, "register.html", map[string]interface{}{
			"errors":   errors,
			"username": username,
			"email":    email,
			"title":    "Registrasi - Portal Ticketing",
		})
		return
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(password1)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Create user
	user := models.User{
		Username: username,
		Email:    email,
		Password: hashedPassword,
		IsActive: true,
	}

	if err := config.DB.Create(&user).Error; err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Add to Portal Users group
	var portalGroup models.Group
	config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
	config.DB.Model(&user).Association("Groups").Append(&portalGroup)

	log.Printf("New user registered: %s", username)

	http.Redirect(w, r, "/login?registered=true", http.StatusSeeOther)
}

// Logout proses logout user
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sess, err := config.Store.Get(r, "session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Destroy session
	sess.Options.MaxAge = -1
	if err := sess.Save(r, w); err != nil {
		log.Printf("Failed to destroy session: %v", err)
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
