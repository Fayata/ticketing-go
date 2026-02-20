package handlers

import (
	"net/http"
	"sort"
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

// ListUsers: Menampilkan daftar semua user
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	filter := r.URL.Query().Get("role")

	var users []models.User
	query := config.DB.Preload("Department").Model(&models.User{})

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
		"template_name": "admin/users_list", // PENTING UNTUK BASE.HTML
		"users":         users,
		"filter":        filter,
		"user":          user,
	})

	// FIXED: Hapus .html
	RenderTemplate(w, "admin/users_list", data)
}

// CreateUserForm: Menambah user baru
func (h *AdminHandler) CreateUserForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var departments []models.Department
		config.DB.Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":         "Tambah User Baru",
			"page_title":    "Tambah User",
			"nav_active":    "admin_users",
			"template_name": "admin/user_form",
			"departments":   departments, 
		})
		RenderTemplate(w, "admin/user_form", data)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")
		role := r.FormValue("role")

		// Ambil Department ID dari form
		deptIDStr := r.FormValue("department_id")
		var departmentID *uint
		if deptIDStr != "" {
			id, _ := strconv.Atoi(deptIDStr)
			uID := uint(id)
			departmentID = &uID
		}

		hashedPassword, _ := utils.HashPassword(password)

		newUser := models.User{
			Username:     username,
			Email:        email,
			Password:     hashedPassword,
			IsActive:     true,
			IsVerified:   true,
			DepartmentID: departmentID, 
		}

		if role == "staff" {
			newUser.IsStaff = true
			// Staff must have a department
			if departmentID == nil {
				http.Error(w, "Staff wajib memiliki departemen", http.StatusBadRequest)
				return
			}
		} else if role == "admin" {
			newUser.IsStaff = true
			newUser.IsSuperAdmin = true
			newUser.DepartmentID = nil // Admin tidak punya department
		}

		if err := config.DB.Create(&newUser).Error; err != nil {
			http.Error(w, "Gagal membuat user: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var portalGroup models.Group
		config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
		config.DB.Model(&newUser).Association("Groups").Append(&portalGroup)

		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
	}
}

// ToggleStatus
func (h *AdminHandler) ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/toggle/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.First(&targetUser, userID).Error; err == nil {
		currentUser := GetUserFromContext(r).(*models.User)
		if currentUser.ID != targetUser.ID {
			targetUser.IsActive = !targetUser.IsActive
			config.DB.Save(&targetUser)
		}
	}
	http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
}

// ToggleStaffRole
func (h *AdminHandler) ToggleStaffRole(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/staff/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.Preload("Department").First(&targetUser, userID).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	// Jangan ubah role untuk Super Admin lewat endpoint ini
	if targetUser.IsSuperAdmin {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	// Jika sudah staff dan GET -> turunkan menjadi user biasa (hapus staff)
	if r.Method == http.MethodGet && targetUser.IsStaff {
		targetUser.IsStaff = false
		targetUser.DepartmentID = nil
		config.DB.Save(&targetUser)
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	// Jika belum staff dan GET -> tampilkan form pilih departemen
	if r.Method == http.MethodGet && !targetUser.IsStaff {
		var departments []models.Department
		config.DB.Order("name ASC").Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":         "Jadikan Staff - " + targetUser.Username,
			"page_title":    "Jadikan Staff",
			"page_subtitle": "Pilih departemen untuk staff baru",
			"nav_active":    "admin_users",
			"template_name": "admin/staff_assign_form",
			"target_user":   targetUser,
			"departments":   departments,
		})

		RenderTemplate(w, "admin/staff_assign_form", data)
		return
	}

	// POST: simpan sebagai staff dengan departemen terpilih
	if r.Method == http.MethodPost && !targetUser.IsStaff {
		_ = r.ParseForm()
		deptIDStr := r.FormValue("department_id")
		if deptIDStr == "" {
			http.Error(w, "Pilih departemen untuk staff", http.StatusBadRequest)
			return
		}
		deptID, _ := strconv.Atoi(deptIDStr)
		deptUID := uint(deptID)

		targetUser.IsStaff = true
		targetUser.DepartmentID = &deptUID
		if err := config.DB.Save(&targetUser).Error; err != nil {
			http.Error(w, "Gagal mengubah user menjadi staff: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
}

// ListDepartments: Menampilkan daftar semua department dengan statistik rating & kontribusi staff
func (h *AdminHandler) ListDepartments(w http.ResponseWriter, r *http.Request) {
	var departments []models.Department
	config.DB.Preload("Tickets").Find(&departments)

	// Hitung statistik rating per departemen
	type deptRatingAgg struct {
		DepartmentID   uint
		AvgRating      float64
		RatingCount    int64
		TicketCount    int64
		CompletedCount int64
	}

	// Aggregate rating & ticket count
	var ratingAggs []struct {
		DepartmentID uint
		AvgRating    float64
		RatingCount  int64
		TicketCount  int64
	}
	config.DB.Table("tickets").
		Select("department_id, AVG(ticket_ratings.rating) AS avg_rating, COUNT(ticket_ratings.id) AS rating_count, COUNT(DISTINCT tickets.id) AS ticket_count").
		Joins("LEFT JOIN ticket_ratings ON ticket_ratings.ticket_id = tickets.id").
		Where("department_id IS NOT NULL").
		Group("department_id").
		Scan(&ratingAggs)

	stats := make(map[uint]deptRatingAgg)
	for _, a := range ratingAggs {
		stats[a.DepartmentID] = deptRatingAgg{
			DepartmentID: a.DepartmentID,
			AvgRating:    a.AvgRating,
			RatingCount:  a.RatingCount,
			TicketCount:  a.TicketCount,
		}
	}

	// Aggregate completed tickets per department (berdasarkan TicketAssignmentHistory)
	var completedAggs []struct {
		DepartmentID uint
		Completed    int64
	}
	config.DB.Table("ticket_assignment_histories AS h").
		Select("t.department_id AS department_id, COUNT(DISTINCT h.ticket_id) AS completed").
		Joins("JOIN tickets t ON t.id = h.ticket_id").
		Where("h.is_completed = ? AND t.department_id IS NOT NULL", true).
		Group("t.department_id").
		Scan(&completedAggs)

	for _, a := range completedAggs {
		s := stats[a.DepartmentID]
		s.DepartmentID = a.DepartmentID
		s.CompletedCount = a.Completed
		stats[a.DepartmentID] = s
	}

	// Urutkan departemen: rating tertinggi dulu, lalu paling banyak tiket selesai, lalu nama
	sort.Slice(departments, func(i, j int) bool {
		di := departments[i]
		dj := departments[j]
		si := stats[di.ID]
		sj := stats[dj.ID]

		// Bandingkan rata-rata rating (descending)
		if si.AvgRating != sj.AvgRating {
			return si.AvgRating > sj.AvgRating
		}
		// Jika sama, bandingkan jumlah tiket selesai
		if si.CompletedCount != sj.CompletedCount {
			return si.CompletedCount > sj.CompletedCount
		}
		// Terakhir, urut alfabetis nama
		return di.Name < dj.Name
	})

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kelola Departemen - Admin Panel",
		"page_title":    "Manajemen Departemen",
		"page_subtitle": "Kelola departemen, rating, dan kontribusi staff",
		"nav_active":    "admin_departments",
		"template_name": "admin/departments_list",
		"departments":   departments,
		"dept_stats":    stats,
	})

	RenderTemplate(w, "admin/departments_list", data)
}

// CreateDepartmentForm: Membuat department baru
func (h *AdminHandler) CreateDepartmentForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := AddBaseData(r, map[string]interface{}{
			"title":         "Tambah Departemen Baru",
			"page_title":    "Tambah Departemen",
			"nav_active":    "admin_departments",
			"template_name": "admin/department_form",
		})
		RenderTemplate(w, "admin/department_form", data)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		name := r.FormValue("name")
		
		if name == "" {
			http.Error(w, "Nama departemen wajib diisi", http.StatusBadRequest)
			return
		}

		newDept := models.Department{
			Name: name,
		}

		if err := config.DB.Create(&newDept).Error; err != nil {
			http.Error(w, "Gagal membuat departemen: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, config.Path("/admin/departments"), http.StatusSeeOther)
	}
}