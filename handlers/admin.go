package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type AdminHandler struct {
	cfg *config.Config
}

func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{cfg: cfg}
}

// ListUsers: Menampilkan daftar semua user (User Biasa & Departemen)
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	filter := r.URL.Query().Get("role") // filter: all, staff, user

	var users []models.User
	query := config.DB.Model(&models.User{})

	if filter == "staff" {
		query = query.Where("is_staff = ?", true)
	} else if filter == "user" {
		query = query.Where("is_staff = ?", false)
	}

	query.Order("created_at DESC").Find(&users)

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kelola Pengguna - Admin Panel",
		"page_title":    "Manajemen User",
		"page_subtitle": "Kelola akun User dan Departemen",
		"nav_active":    "admin_users",
		"users":         users,
		"filter":        filter,
		"user":          user,
	})

	RenderTemplate(w, "admin/users_list.html", data) // Perhatikan path foldernya
}

// CreateUser: Menambah user baru (bisa set jadi Staff/Admin langsung)
func (h *AdminHandler) CreateUserForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := AddBaseData(r, map[string]interface{}{
			"title":      "Tambah User Baru",
			"nav_active": "admin_users",
		})
		RenderTemplate(w, "admin/user_form.html", data)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")
		role := r.FormValue("role") // "user", "staff", "admin"

		hashedPassword, _ := utils.HashPassword(password)

		newUser := models.User{
			Username:   username,
			Email:      email,
			Password:   hashedPassword,
			IsActive:   true,
			IsVerified: true,
		}

		// Set Role
		if role == "staff" {
			newUser.IsStaff = true
		} else if role == "admin" {
			newUser.IsStaff = true
			newUser.IsSuperAdmin = true
		}

		if err := config.DB.Create(&newUser).Error; err != nil {
			// Handle error duplicate dll
			http.Error(w, "Gagal membuat user: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Assign Group Portal Users (wajib biar bisa login)
		var portalGroup models.Group
		config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
		config.DB.Model(&newUser).Association("Groups").Append(&portalGroup)

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

// ToggleStatus: Mengaktifkan/Menonaktifkan User
func (h *AdminHandler) ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/toggle/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.First(&targetUser, userID).Error; err == nil {
		// Proteksi: Jangan nonaktifkan diri sendiri
		currentUser := GetUserFromContext(r).(*models.User)
		if currentUser.ID != targetUser.ID {
			targetUser.IsActive = !targetUser.IsActive
			config.DB.Save(&targetUser)
		}
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// MakeStaff: Shortcut untuk mengubah user biasa jadi staff departemen
func (h *AdminHandler) ToggleStaffRole(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/staff/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.First(&targetUser, userID).Error; err == nil {
		targetUser.IsStaff = !targetUser.IsStaff
		config.DB.Save(&targetUser)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
