package handlers

import (
	"log"
	"net/http"
	"strings"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type SettingsHandler struct {
	cfg *config.Config
}

func NewSettingsHandler(cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{cfg: cfg}
}

// HandleSettings handles both GET and POST for settings
func (h *SettingsHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ShowSettings(w, r)
	case http.MethodPost:
		// Check which form is submitted
		r.ParseForm()
		if r.FormValue("update_profile") != "" {
			h.UpdateProfile(w, r)
		} else if r.FormValue("change_password") != "" {
			h.ChangePassword(w, r)
		} else {
			http.Error(w, "Bad request", http.StatusBadRequest)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ShowSettings menampilkan halaman settings
func (h *SettingsHandler) ShowSettings(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Get flash messages from query params (simplified)
	successMsg := r.URL.Query().Get("success")
	errorMsg := r.URL.Query().Get("error")

	data := AddBaseData(r, map[string]interface{}{
		"title": "Settings - Portal Ticketing",
		"user":  user,
	})

	if successMsg != "" {
		data["success"] = successMsg
	}

	if errorMsg != "" {
		data["error"] = errorMsg
	}
	h.renderSettingsPage(w, r, data)
}

// UpdateProfile update profil user
func (h *SettingsHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))

	// Validasi
	errors := make(map[string]string)

	if username == "" {
		errors["username"] = "Username wajib diisi"
	}
	if email == "" {
		errors["email"] = "Email wajib diisi"
	}

	// Check username exists (exclude current user)
	var existingUser models.User
	if username != "" {
		if err := config.DB.Where("username = ? AND id != ?", username, user.ID).First(&existingUser).Error; err == nil {
			errors["username"] = "Username sudah digunakan"
		}
	}

	// Check email exists (exclude current user)
	if email != "" {
		if err := config.DB.Where("email = ? AND id != ?", email, user.ID).First(&existingUser).Error; err == nil {
			errors["email"] = "Email sudah terdaftar"
		}
	}

	if len(errors) > 0 {
		data := map[string]interface{}{
			"errors":        errors,
			"form_username": username,
			"form_email":    email,
			"form_first":    firstName,
			"form_last":     lastName,
		}
		h.renderSettingsPage(w, r, data)
		return
	}

	// Update user
	user.Username = username
	user.Email = email
	user.FirstName = firstName
	user.LastName = lastName

	if err := config.DB.Save(user).Error; err != nil {
		log.Printf("Failed to update user: %v", err)
		data := map[string]interface{}{
			"errors": map[string]string{
				"__all__": "Gagal memperbarui profil. Silakan coba lagi.",
			},
			"form_username": username,
			"form_email":    email,
			"form_first":    firstName,
			"form_last":     lastName,
		}
		h.renderSettingsPage(w, r, data)
		return
	}

	// Update session username
	sess, _ := config.Store.Get(r, "session")
	sess.Values["username"] = username
	sess.Save(r, w)

	log.Printf("Profile updated for user: %s", username)

	http.Redirect(w, r, "/settings?success=Profil+berhasil+diperbarui", http.StatusSeeOther)
}

// ChangePassword mengubah password user
func (h *SettingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	oldPassword := r.FormValue("old_password")
	newPassword1 := r.FormValue("new_password1")
	newPassword2 := r.FormValue("new_password2")

	// Validasi
	errors := make(map[string]string)

	if oldPassword == "" {
		errors["old_password"] = "Password lama wajib diisi"
	}
	if newPassword1 == "" {
		errors["new_password1"] = "Password baru wajib diisi"
	}
	if newPassword2 == "" {
		errors["new_password2"] = "Konfirmasi password baru wajib diisi"
	}

	if len(errors) > 0 {
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors": errors,
		})
		return
	}

	// Check old password
	if !utils.CheckPasswordHash(oldPassword, user.Password) {
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors": map[string]string{
				"old_password": "Password lama tidak sesuai",
			},
		})
		return
	}

	// Check new passwords match
	if newPassword1 != newPassword2 {
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors": map[string]string{
				"new_password2": "Password baru tidak cocok",
			},
		})
		return
	}

	// Validate new password length
	if len(newPassword1) < 8 {
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors": map[string]string{
				"new_password1": "Password minimal 8 karakter",
			},
		})
		return
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(newPassword1)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		http.Redirect(w, r, "/settings?error=Gagal+mengubah+password", http.StatusSeeOther)
		return
	}

	// Update password
	user.Password = hashedPassword
	if err := config.DB.Save(user).Error; err != nil {
		log.Printf("Failed to update password: %v", err)
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors": map[string]string{
				"__all__": "Gagal mengubah password. Silakan coba lagi.",
			},
		})
		return
	}

	log.Printf("Password changed for user: %s", user.Username)

	http.Redirect(w, r, "/settings?success=Password+berhasil+diubah", http.StatusSeeOther)
}

func (h *SettingsHandler) renderSettingsPage(w http.ResponseWriter, r *http.Request, data map[string]interface{}) {
	if data == nil {
		data = make(map[string]interface{})
	}

	data = AddBaseData(r, data)

	if data["title"] == nil {
		data["title"] = "Settings - Portal Ticketing"
	}
	if data["page_title"] == nil {
		data["page_title"] = "Pengaturan Akun"
	}
	if data["page_subtitle"] == nil {
		data["page_subtitle"] = "Kelola informasi profil dan keamanan akun Anda"
	}
	if data["template_name"] == nil {
		data["template_name"] = "tickets/settings"
	}

	RenderTemplate(w, "tickets/settings.html", data)
}
