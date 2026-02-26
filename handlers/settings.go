package handlers

import (
	"log"
	"net/http"
	"strings"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
)

type SettingsHandler struct {
	cfg              *config.Config
	settingsService  *services.SettingsService
}

func NewSettingsHandler(cfg *config.Config, settingsService *services.SettingsService) *SettingsHandler {
	return &SettingsHandler{cfg: cfg, settingsService: settingsService}
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

	result, _ := h.settingsService.UpdateProfile(user.ID, username, email, firstName, lastName)
	if result != nil && len(result.Errors) > 0 {
		h.renderSettingsPage(w, r, map[string]interface{}{
			"errors":        result.Errors,
			"form_username": username,
			"form_email":    email,
			"form_first":    firstName,
			"form_last":     lastName,
		})
		return
	}
	sess, _ := config.Store.Get(r, "session")
	sess.Values["username"] = username
	sess.Save(r, w)
	log.Printf("Profile updated for user: %s", username)
	http.Redirect(w, r, config.Path("/settings")+"?success=Profil+berhasil+diperbarui", http.StatusSeeOther)
}

// ChangePassword mengubah password user
func (h *SettingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	result, _ := h.settingsService.ChangePassword(user.ID, r.FormValue("old_password"), r.FormValue("new_password1"), r.FormValue("new_password2"))
	if result != nil && len(result.Errors) > 0 {
		h.renderSettingsPage(w, r, map[string]interface{}{"errors": result.Errors})
		return
	}
	log.Printf("Password changed for user: %s", user.Username)
	http.Redirect(w, r, config.Path("/settings")+"?success=Password+berhasil+diubah", http.StatusSeeOther)
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

	RenderTemplate(w, "tickets/settings", data)
}
