package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

// SetEmailHandler menangani halaman set/verifikasi email untuk akun tanpa email.
type SetEmailHandler struct {
	cfg          *config.Config
	emailService *utils.EmailService
	jwtService   *utils.JWTService
}

func NewSetEmailHandler(cfg *config.Config, emailService *utils.EmailService, jwtService *utils.JWTService) *SetEmailHandler {
	return &SetEmailHandler{cfg: cfg, emailService: emailService, jwtService: jwtService}
}

// HandleSetEmail menangani GET (tampilkan form) dan POST (simpan email + kirim verifikasi).
func (h *SetEmailHandler) HandleSetEmail(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	if r.Method == http.MethodGet {
		data := AddBaseData(r, map[string]interface{}{
			"title":         "Lengkapi Akun - Set Email",
			"page_title":    "Lengkapi Akun",
			"template_name": "tickets/set_email",
			"error":         r.URL.Query().Get("error"),
			"success":       r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "tickets/set_email", data)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		http.Redirect(w, r, config.Path("/set-email")+"?error=Email+wajib+diisi", http.StatusSeeOther)
		return
	}

	// Cek duplikat email
	var existing models.User
	if err := config.DB.Where("email = ? AND id != ?", email, user.ID).First(&existing).Error; err == nil {
		http.Redirect(w, r, config.Path("/set-email")+"?error=Email+sudah+digunakan+akun+lain", http.StatusSeeOther)
		return
	}

	// Simpan email dan tandai belum diverifikasi
	if err := config.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"email":       email,
		"is_verified": false,
	}).Error; err != nil {
		log.Printf("[SetEmail] Failed to set email for user %d: %v", user.ID, err)
		http.Redirect(w, r, config.Path("/set-email")+"?error=Gagal+menyimpan+email", http.StatusSeeOther)
		return
	}

	// Kirim email verifikasi secara async
	go func() {
		token, err := h.jwtService.GenerateToken(user.ID, "verify_email", 24*time.Hour)
		if err != nil {
			log.Printf("[SetEmail] Failed to generate verification token for user %d: %v", user.ID, err)
			return
		}
		link := fmt.Sprintf("%s/verify-email?token=%s", h.cfg.BaseURL, token)
		body := fmt.Sprintf(
			"Halo %s,\n\nKlik link berikut untuk memverifikasi email Anda:\n\n%s\n\nLink berlaku 24 jam.",
			user.Username, link,
		)
		if err := h.emailService.SendMail(email, "Verifikasi Email - Portal Ticketing", body); err != nil {
			log.Printf("[SetEmail] Failed to send verification email to %s: %v", email, err)
		}
	}()

	http.Redirect(w, r, config.Path("/set-email")+"?success=Email+berhasil+disimpan.+Cek+inbox+untuk+verifikasi", http.StatusSeeOther)
}