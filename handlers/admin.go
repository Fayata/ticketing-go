package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

const (
	kbUploadDir       = "static/uploads/kb"
	maxKBImageSize    = 5 << 20 // 5 MB
	kbImageLayoutFull = "full"
	kbImageLayoutHalf = "half"
	kbImageLayoutThumb = "thumb"
)

type AdminHandler struct {
	cfg                *config.Config
	adminDashService   *services.AdminDashboardService
	aiService          *services.AIService
	adminSearch        *services.AdminSearchService
	adminReportService *services.AdminReportService
}

func NewAdminHandler(cfg *config.Config, adminDashService *services.AdminDashboardService, aiService *services.AIService, adminSearch *services.AdminSearchService, adminReportService ...*services.AdminReportService) *AdminHandler {
	var reportSvc *services.AdminReportService
	if len(adminReportService) > 0 && adminReportService[0] != nil {
		reportSvc = adminReportService[0]
	} else {
		reportSvc = services.NewAdminReportService()
	}
	return &AdminHandler{
		cfg:                cfg,
		adminDashService:   adminDashService,
		aiService:          aiService,
		adminSearch:        adminSearch,
		adminReportService: reportSvc,
	}
}

// ShowAdminDashboard menampilkan halaman dashboard admin: KPI, grafik, tiket terbaru, menunggu terlama.
func (h *AdminHandler) ShowAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.adminDashService == nil {
		http.Error(w, "Dashboard service not configured", http.StatusInternalServerError)
		return
	}
	dash, err := h.adminDashService.GetAdminDashboardData()
	if err != nil {
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}
	trendJSON, _ := json.Marshal(dash.TrendData)
	statusJSON, _ := json.Marshal(dash.StatusData)
	deptJSON, _ := json.Marshal(dash.DeptData)
	priorityJSON, _ := json.Marshal(dash.PriorityData)
	ratingJSON, _ := json.Marshal(dash.RatingData)
	staffJSON, _ := json.Marshal(dash.StaffData)
	data := AddBaseData(r, map[string]interface{}{
		"title":             "Dashboard Admin — Ticketing",
		"page_title":        "Dashboard",
		"page_subtitle":     "Sistem Ticketing Admin",
		"nav_active":        "admin_dashboard",
		"template_name":     "admin/admin_dashboard",
		"dashboard":         dash,
		"trend_data_json":   template.JS(trendJSON),
		"status_data_json":  template.JS(statusJSON),
		"dept_data_json":    template.JS(deptJSON),
		"priority_data_json": template.JS(priorityJSON),
		"rating_data_json":  template.JS(ratingJSON),
		"staff_data_json":   template.JS(staffJSON),
	})
	RenderTemplate(w, "admin/admin_dashboard", data)
}

// GetLiveDashboardAPI returns JSON of admin dashboard metrics and waiting tickets for real-time sync.
func (h *AdminHandler) GetLiveDashboardAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.adminDashService == nil {
		http.Error(w, "Dashboard service not configured", http.StatusInternalServerError)
		return
	}

	dash, err := h.adminDashService.GetAdminDashboardData()
	if err != nil {
		http.Error(w, "Failed to load dashboard data", http.StatusInternalServerError)
		return
	}

	type waitingItemJSON struct {
		ID              uint   `json:"id"`
		TicketNumber    string `json:"ticket_number"`
		Title           string `json:"title"`
		Department      string `json:"department"`
		Priority        string `json:"priority"`
		WaitingDuration string `json:"waiting_duration"`
		SLABadgeClass   string `json:"sla_badge_class"`
		SLABadgeLabel   string `json:"sla_badge_label"`
	}

	waitingList := make([]waitingItemJSON, 0, len(dash.WaitingLongest))
	for _, item := range dash.WaitingLongest {
		if item.Ticket == nil {
			continue
		}
		deptName := "—"
		if item.Ticket.Department != nil {
			deptName = item.Ticket.Department.Name
		}
		sla := item.Ticket.GetSLABadgeInfo()
		waitingList = append(waitingList, waitingItemJSON{
			ID:              item.Ticket.ID,
			TicketNumber:    item.Ticket.GetTicketNumber(),
			Title:           item.Ticket.Title,
			Department:      deptName,
			Priority:        string(item.Ticket.Priority),
			WaitingDuration: fmt.Sprintf("%d hari", item.Days),
			SLABadgeClass:   sla.Class,
			SLABadgeLabel:   sla.Label,
		})
	}

	type recentTicketJSON struct {
		ID           uint   `json:"id"`
		TicketNumber string `json:"ticket_number"`
		Title        string `json:"title"`
		Status       string `json:"status"`
		Priority     string `json:"priority"`
		Department   string `json:"department"`
		CreatedAt    string `json:"created_at"`
	}

	recentList := make([]recentTicketJSON, 0, len(dash.RecentTickets))
	for _, t := range dash.RecentTickets {
		deptName := "—"
		if t.Department != nil {
			deptName = t.Department.Name
		}
		recentList = append(recentList, recentTicketJSON{
			ID:           t.ID,
			TicketNumber: t.GetTicketNumber(),
			Title:        t.Title,
			Status:       string(t.Status),
			Priority:     string(t.Priority),
			Department:   deptName,
			CreatedAt:    t.CreatedAt.Format("02 Jan 15:04"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"kpi": map[string]interface{}{
			"WaitingCount":        dash.WaitingCount,
			"InProgressCount":     dash.InProgressCount,
			"ClosedTodayCount":    dash.ClosedTodayCount,
			"AvgRating":           dash.AvgRating,
			"RatedCount":          dash.RatedCount,
			"TotalTicketsMonth":   dash.TotalTicketsMonth,
			"TotalUsersActive":    dash.TotalUsersActive,
			"StaffActiveCount":    dash.StaffActiveCount,
			"UnratedCount":        dash.UnratedCount,
			"TrendWaitingPct":     dash.TrendWaitingPct,
			"TrendProgressPct":    dash.TrendProgressPct,
			"TrendClosedTodayPct": dash.TrendClosedTodayPct,
			"TrendAvgRatingPct":   dash.TrendAvgRatingPct,
		},
		"waiting_tickets": waitingList,
		"recent_tickets":  recentList,
		"server_time":     time.Now().Format(time.RFC3339),
	})
}

// ListUsers menampilkan daftar user dengan filter role (user/staff) untuk admin.
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	filter := r.URL.Query().Get("role")

	var users []models.User
	query := config.DB.Preload("Department.Company").Model(&models.User{})

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
		"template_name": "admin/users_list",
		"users":         users,
		"filter":        filter,
		"user":          user,
		"success":       r.URL.Query().Get("success"),
		"error":         r.URL.Query().Get("error"),
	})

	RenderTemplate(w, "admin/users_list", data)
}

// CreateUserForm menampilkan form tambah user (GET) atau menyimpan user baru (POST).
func (h *AdminHandler) CreateUserForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var companies []models.Company
		config.DB.Where("is_active = ?", true).Preload("Departments").Order("name ASC").Find(&companies)

		var departments []models.Department
		config.DB.Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":         "Tambah User Baru",
			"page_title":    "Tambah User",
			"nav_active":    "admin_users",
			"template_name": "admin/user_form",
			"companies":     companies,
			"departments":   departments,
			"error":         r.URL.Query().Get("error"),
			"success":       r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/user_form", data)
		return
	}

	if r.Method == http.MethodPost {
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")
		role := strings.TrimSpace(r.FormValue("role"))
		if role == "" {
			role = "user"
		}

		if username == "" || len(username) < 3 {
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Username+minimal+3+karakter", http.StatusSeeOther)
			return
		}

		if len(password) < 6 {
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Password+minimal+6+karakter", http.StatusSeeOther)
			return
		}

		// Cek apakah username sudah dipakai
		var existingUser models.User
		if err := config.DB.Where("LOWER(username) = LOWER(?)", username).First(&existingUser).Error; err == nil {
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Username+sudah+digunakan", http.StatusSeeOther)
			return
		}

		// Cek duplikat email hanya jika email diisi
		var emailPtr *string
		if email != "" {
			if err := config.DB.Where("LOWER(email) = LOWER(?)", email).First(&existingUser).Error; err == nil {
				http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Email+sudah+terdaftar", http.StatusSeeOther)
				return
			}
			emailPtr = &email
		}

		deptIDStr := strings.TrimSpace(r.FormValue("department_id"))
		var departmentID *uint
		if deptIDStr != "" && deptIDStr != "0" {
			id, err := strconv.Atoi(deptIDStr)
			if err == nil && id > 0 {
				uID := uint(id)
				departmentID = &uID
			}
		}

		if role == "staff" && departmentID == nil {
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Staff+wajib+memilih+departemen", http.StatusSeeOther)
			return
		}

		hashedPassword, err := utils.HashPassword(password)
		if err != nil {
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Gagal+memproses+password", http.StatusSeeOther)
			return
		}

		// Akun dibuat admin: langsung verified (email opsional, user set sendiri nanti)
		newUser := models.User{
			Username:     username,
			Email:        emailPtr,
			Password:     hashedPassword,
			IsActive:     true,
			IsVerified:   true,
			DepartmentID: departmentID,
		}

		if role == "staff" {
			newUser.IsStaff = true
		} else if role == "admin" {
			newUser.IsStaff = true
			newUser.IsSuperAdmin = true
			newUser.DepartmentID = nil
		}

		if err := config.DB.Create(&newUser).Error; err != nil {
			log.Printf("[Security][Admin] Failed to create user: %v", err)
			http.Redirect(w, r, config.Path("/admin/users/create")+"?error=Gagal+menyimpan+user", http.StatusSeeOther)
			return
		}

		var portalGroup models.Group
		config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
		config.DB.Model(&newUser).Association("Groups").Append(&portalGroup)

		http.Redirect(w, r, config.Path("/admin/users")+"?success=User+berhasil+dibuat", http.StatusSeeOther)
	}
}

// ToggleUserStatus mengaktifkan/nonaktifkan user (untuk admin).
func (h *AdminHandler) ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/admin/users/toggle/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.First(&targetUser, userID).Error; err == nil {
		currentUser := GetUserFromContext(r).(*models.User)
		if currentUser.ID != targetUser.ID {
			log.Printf("[Security][Admin] User %d toggling status of user %d", currentUser.ID, targetUser.ID)
			targetUser.IsActive = !targetUser.IsActive
			config.DB.Save(&targetUser)
		}
	}
	http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
}

// ToggleStaffRole mengubah user jadi staff (pilih dept) atau turunkan jadi user biasa.
func (h *AdminHandler) ToggleStaffRole(w http.ResponseWriter, r *http.Request) {
	userID := parseIDFromPath(r.URL.Path)

	var targetUser models.User
	if err := config.DB.Preload("Department").First(&targetUser, userID).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	if targetUser.IsSuperAdmin {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost && targetUser.IsStaff {
		currentUser := GetUserFromContext(r).(*models.User)
		log.Printf("[Security][Admin] User %d changing staff role of user %d", currentUser.ID, targetUser.ID)
		targetUser.IsStaff = false
		targetUser.DepartmentID = nil
		config.DB.Save(&targetUser)
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet && !targetUser.IsStaff {
		var companies []models.Company
		config.DB.Where("is_active = ?", true).Preload("Departments").Order("name ASC").Find(&companies)

		var departments []models.Department
		config.DB.Order("name ASC").Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":         "Jadikan Staff - " + targetUser.Username,
			"page_title":    "Jadikan Staff",
			"page_subtitle": "Pilih departemen untuk staff baru",
			"nav_active":    "admin_users",
			"template_name": "admin/staff_assign_form",
			"target_user":   targetUser,
			"companies":     companies,
			"departments":   departments,
		})

		RenderTemplate(w, "admin/staff_assign_form", data)
		return
	}

	if r.Method == http.MethodPost && !targetUser.IsStaff {
		_ = r.ParseForm()
		deptIDStr := r.FormValue("department_id")
		if deptIDStr == "" {
			http.Error(w, "Pilih departemen untuk staff", http.StatusBadRequest)
			return
		}
		deptID, _ := strconv.Atoi(deptIDStr)
		deptUID := uint(deptID)

		currentUser := GetUserFromContext(r).(*models.User)
		log.Printf("[Security][Admin] User %d changing staff role of user %d", currentUser.ID, targetUser.ID)
		targetUser.IsStaff = true
		targetUser.DepartmentID = &deptUID
		if err := config.DB.Save(&targetUser).Error; err != nil {
			log.Printf("[Security][Admin] Failed to change user to staff: %v", err)
			http.Error(w, "Gagal mengubah user menjadi staff. Silakan coba lagi.", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
}

// ListDepartments menampilkan daftar departemen dengan statistik rating dan tiket selesai.
func (h *AdminHandler) ListDepartments(w http.ResponseWriter, r *http.Request) {
	var departments []models.Department
	config.DB.Preload("Tickets").Preload("Company").Find(&departments)

	type deptRatingAgg struct {
		DepartmentID   uint
		AvgRating      float64
		RatingCount    int64
		TicketCount    int64
		CompletedCount int64
	}

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

	sort.Slice(departments, func(i, j int) bool {
		di := departments[i]
		dj := departments[j]
		si := stats[di.ID]
		sj := stats[dj.ID]

		if si.AvgRating != sj.AvgRating {
			return si.AvgRating > sj.AvgRating
		}
		if si.CompletedCount != sj.CompletedCount {
			return si.CompletedCount > sj.CompletedCount
		}
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
		"error":         r.URL.Query().Get("error"),
		"success":       r.URL.Query().Get("success"),
	})

	RenderTemplate(w, "admin/departments_list", data)
}

// CreateDepartmentForm menampilkan form tambah departemen (GET) atau menyimpan (POST).
func (h *AdminHandler) CreateDepartmentForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var companies []models.Company
		config.DB.Where("is_active = ?", true).Order("name ASC").Find(&companies)

		data := AddBaseData(r, map[string]interface{}{
			"title":         "Tambah Departemen Baru",
			"page_title":    "Tambah Departemen",
			"nav_active":    "admin_departments",
			"template_name": "admin/department_form",
			"companies":     companies,
			"error":         r.URL.Query().Get("error"),
			"success":       r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/department_form", data)
		return
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		companyIDStr := strings.TrimSpace(r.FormValue("company_id"))

		if name == "" {
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Nama departemen wajib diisi"), http.StatusSeeOther)
			return
		}

		if companyIDStr == "" {
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Perusahaan wajib dipilih"), http.StatusSeeOther)
			return
		}

		compID, err := strconv.Atoi(companyIDStr)
		if err != nil || compID <= 0 {
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Perusahaan tidak valid"), http.StatusSeeOther)
			return
		}

		var comp models.Company
		if err := config.DB.First(&comp, compID).Error; err != nil {
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Perusahaan tidak ditemukan"), http.StatusSeeOther)
			return
		}

		if !comp.IsActive {
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Perusahaan yang dipilih sedang nonaktif"), http.StatusSeeOther)
			return
		}

		uCompID := uint(compID)
		newDept := models.Department{
			Name:      name,
			CompanyID: &uCompID,
		}

		if err := config.DB.Create(&newDept).Error; err != nil {
			log.Printf("[Security][Admin] Failed to create department: %v", err)
			http.Redirect(w, r, config.Path("/admin/departments/create")+"?error="+url.QueryEscape("Gagal membuat departemen. Silakan coba lagi."), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, config.Path("/admin/departments")+"?success="+url.QueryEscape("Departemen berhasil dibuat"), http.StatusSeeOther)
		return
	}
}

// --- Knowledge Base Admin (Staff & SuperAdmin) ---

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	for _, r := range s {
		if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			s = strings.ReplaceAll(s, string(r), "")
		}
	}
	return s
}

// parseKBIDFromPath mengambil ID dari segment terakhir path (r.URL.Path sudah tanpa base path dari middleware).
// Contoh: "/admin/knowledge-base/categories/edit/5" -> 5
func parseKBIDFromPath(path string) int {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0
	}
	id, _ := strconv.Atoi(parts[len(parts)-1])
	return id
}

func (h *AdminHandler) ListKBAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userVal := GetUserFromContext(r)
	var user *models.User
	if userVal != nil {
		if u, ok := userVal.(*models.User); ok {
			user = u
		}
	}
	var categories []models.KBCategory
	config.DB.Order("sort_order ASC, name ASC").Find(&categories)
	var articles []models.KBArticle
	config.DB.Preload("Category").Order("updated_at DESC").Find(&articles)
	if categories == nil {
		categories = []models.KBCategory{}
	}
	if articles == nil {
		articles = []models.KBArticle{}
	}
	var messages []string
	if s := r.URL.Query().Get("success"); s != "" {
		messages = append(messages, s)
	}
	if e := r.URL.Query().Get("error"); e != "" {
		messages = append(messages, "Error: "+e)
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":          "Kelola Knowledge Base",
		"page_title":     "Kelola Knowledge Base",
		"page_subtitle":  "Kategori dan artikel",
		"nav_active":     "admin_kb",
		"template_name":  "admin/kb_list",
		"user":           user,
		"categories":     categories,
		"articles":       articles,
		"messages":       messages,
	})
	if user != nil && user.IsStaff && !user.IsSuperAdmin {
		data["nav_active"] = "kb_admin"
		data["template_name"] = "department_kb_list"
		RenderTemplate(w, "department_kb_list", data)
		return
	}
	RenderTemplate(w, "admin/kb_list", data)
}

func (h *AdminHandler) CreateKBCategoryForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	errMsg := r.URL.Query().Get("error")
	userVal := GetUserFromContext(r)
	var user *models.User
	if userVal != nil {
		if u, ok := userVal.(*models.User); ok {
			user = u
		}
	}
	data := AddBaseData(r, map[string]interface{}{
		"title":         "Tambah Kategori KB",
		"page_title":    "Tambah Kategori",
		"nav_active":    "admin_kb",
		"template_name": "admin/kb_category_form",
		"error":         errMsg,
	})
	if user != nil && user.IsStaff && !user.IsSuperAdmin {
		data["nav_active"] = "kb_admin"
		data["template_name"] = "department_kb_category_form"
		RenderTemplate(w, "department_kb_category_form", data)
		return
	}
	RenderTemplate(w, "admin/kb_category_form", data)
}

func (h *AdminHandler) CreateKBCategoryPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, config.Path("/admin/knowledge-base"), http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	colorClass := strings.TrimSpace(r.FormValue("color_class"))
	if colorClass == "" {
		colorClass = "green"
	}
	if name == "" {
		http.Redirect(w, r, config.Path("/admin/knowledge-base/categories/create")+"?error=Nama+wajib+diisi", http.StatusSeeOther)
		return
	}
	slug := slugify(name)
	var existing models.KBCategory
	if config.DB.Where("slug = ?", slug).First(&existing).Error == nil {
		slug = slug + "-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	cat := models.KBCategory{
		Name:        name,
		Slug:        slug,
		Description: desc,
		Icon:        icon,
		ColorClass:  colorClass,
	}
	if err := config.DB.Create(&cat).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base/categories/create")+"?error=Gagal+menyimpan", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Kategori+berhasil+ditambah", http.StatusSeeOther)
}

func (h *AdminHandler) CreateKBArticleForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var categories []models.KBCategory
	config.DB.Order("sort_order ASC, name ASC").Find(&categories)
	errMsg := r.URL.Query().Get("error")
	userVal := GetUserFromContext(r)
	var user *models.User
	if userVal != nil {
		if u, ok := userVal.(*models.User); ok {
			user = u
		}
	}
	data := AddBaseData(r, map[string]interface{}{
		"title":         "Tambah Artikel KB",
		"page_title":    "Tambah Artikel",
		"nav_active":    "admin_kb",
		"template_name": "admin/kb_article_form",
		"categories":    categories,
		"error":         errMsg,
	})
	if user != nil && user.IsStaff && !user.IsSuperAdmin {
		data["nav_active"] = "kb_admin"
		data["template_name"] = "department_kb_article_form"
		RenderTemplate(w, "department_kb_article_form", data)
		return
	}
	RenderTemplate(w, "admin/kb_article_form", data)
}

// parseArticleSectionsFromRequest parses multipart form untuk section_0_subtitle, section_0_content, section_0_image, section_0_layout, ...
// existingImagePaths: untuk edit, map index -> path yang sudah ada (jika tidak ada upload baru).
func parseArticleSectionsFromRequest(r *http.Request, articleID uint, existingImagePaths map[int]string) (string, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return "[]", nil
	}
	form := r.MultipartForm
	if form == nil {
		return "[]", nil
	}
	var indices []int
	for key := range form.Value {
		if strings.HasPrefix(key, "section_") && strings.HasSuffix(key, "_subtitle") {
			mid := strings.TrimPrefix(strings.TrimSuffix(key, "_subtitle"), "section_")
			if i, err := strconv.Atoi(mid); err == nil {
				indices = append(indices, i)
			}
		}
	}
	sort.Ints(indices)
	if len(indices) == 0 {
		return "[]", nil
	}
	_ = os.MkdirAll(kbUploadDir, 0755)
	var sections []models.KBArticleSection
	for _, i := range indices {
		prefix := "section_" + strconv.Itoa(i) + "_"
		subtitle := strings.TrimSpace(firstFormValue(form.Value, prefix+"subtitle"))
		content := firstFormValue(form.Value, prefix+"content")
		layout := strings.TrimSpace(firstFormValue(form.Value, prefix+"layout"))
		if layout != kbImageLayoutHalf && layout != kbImageLayoutThumb {
			layout = kbImageLayoutFull
		}
		imagePath := existingImagePaths[i]
		if p := firstFormValue(form.Value, prefix+"image_path"); p != "" {
			imagePath = strings.TrimSpace(p)
		}
		if fhs := form.File[prefix+"image"]; len(fhs) > 0 {
			fh := fhs[0]
			if fh.Size > 0 && fh.Size <= maxKBImageSize {
				ext := strings.ToLower(filepath.Ext(fh.Filename))
				if ext == "" {
					ext = ".jpg"
				}
				allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
				if allowed[ext] {
					baseID := int(articleID)
					name := strconv.Itoa(baseID) + "_" + strconv.Itoa(i) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10) + ext
					dstPath := filepath.Join(kbUploadDir, name)
					if saveUploadedFile(fh, dstPath) == nil {
						imagePath = "uploads/kb/" + name
					}
				}
			}
		}
		sections = append(sections, models.KBArticleSection{
			Subtitle:    subtitle,
			Content:      content,
			ImagePath:    imagePath,
			ImageLayout:  layout,
		})
	}
	b, err := json.Marshal(sections)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}

func firstFormValue(v map[string][]string, key string) string {
	if v == nil {
		return ""
	}
	vals := v[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// saveUploadedFile menyimpan file upload dan memvalidasi magic bytes untuk keamanan.
func saveUploadedFile(fh *multipart.FileHeader, dstPath string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// [Security] Validasi magic bytes — pastikan file benar-benar gambar
	header := make([]byte, 512)
	n, err := src.Read(header)
	if err != nil {
		return fmt.Errorf("[Security][Upload] Failed to read file header: %w", err)
	}
	header = header[:n]

	contentType := http.DetectContentType(header)
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}
	if !allowedTypes[contentType] {
		log.Printf("[Security][Upload] BLOCKED: file %q has content-type %q, not an allowed image", fh.Filename, contentType)
		return fmt.Errorf("file type not allowed: %s", contentType)
	}

	// Reset reader to beginning
	if seeker, ok := src.(io.ReadSeeker); ok {
		seeker.Seek(0, io.SeekStart)
	} else {
		src.Close()
		src, _ = fh.Open()
	}

	// [Security] Sanitize filename — hapus path traversal
	cleanName := filepath.Base(dstPath)
	cleanDir := filepath.Dir(dstPath)
	safePath := filepath.Join(cleanDir, cleanName)

	log.Printf("[Security][Upload] Saving validated file: %s (type: %s, size: %d)", safePath, contentType, fh.Size)

	dst, err := os.Create(safePath)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func (h *AdminHandler) CreateKBArticlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, config.Path("/admin/knowledge-base"), http.StatusSeeOther)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	content := r.FormValue("content")
	categoryIDStr := r.FormValue("category_id")
	readTimeStr := strings.TrimSpace(r.FormValue("read_time_minutes"))
	if title == "" || categoryIDStr == "" {
		http.Redirect(w, r, config.Path("/admin/knowledge-base/articles/create")+"?error=Judul+dan+kategori+wajib", http.StatusSeeOther)
		return
	}
	sectionsJSON, _ := parseArticleSectionsFromRequest(r, 0, nil)
	catID, _ := strconv.Atoi(categoryIDStr)
	readTime, _ := strconv.Atoi(readTimeStr)
	slug := slugify(title)
	var existing models.KBArticle
	if config.DB.Where("slug = ?", slug).First(&existing).Error == nil {
		slug = slug + "-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	art := models.KBArticle{
		CategoryID:      uint(catID),
		Title:           title,
		Slug:            slug,
		Content:         content,
		Sections:        sectionsJSON,
		ReadTimeMinutes: readTime,
		Published:      true,
	}
	if err := config.DB.Create(&art).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base/articles/create")+"?error=Gagal+menyimpan", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Artikel+berhasil+ditambah", http.StatusSeeOther)
}

// EditKBCategory menampilkan form edit (GET) atau menyimpan perubahan (POST). Path: .../categories/edit/<id>
func (h *AdminHandler) EditKBCategory(w http.ResponseWriter, r *http.Request) {
	id := parseKBIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=ID+kategori+tidak+valid", http.StatusSeeOther)
		return
	}
	var cat models.KBCategory
	if config.DB.First(&cat, id).Error != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Kategori+tidak+ditemukan", http.StatusSeeOther)
		return
	}
	userVal := GetUserFromContext(r)
	var user *models.User
	if userVal != nil {
		if u, ok := userVal.(*models.User); ok {
			user = u
		}
	}

	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("name"))
		desc := strings.TrimSpace(r.FormValue("description"))
		icon := strings.TrimSpace(r.FormValue("icon"))
		colorClass := strings.TrimSpace(r.FormValue("color_class"))
		if colorClass == "" {
			colorClass = "green"
		}
		if name == "" {
			http.Redirect(w, r, config.Path("/admin/knowledge-base/categories/edit/")+strconv.Itoa(id)+"?error=Nama+wajib+diisi", http.StatusSeeOther)
			return
		}
		slug := slugify(name)
		if slug != cat.Slug {
			var existing models.KBCategory
			if config.DB.Where("slug = ? AND id != ?", slug, id).First(&existing).Error == nil {
				slug = slug + "-" + strconv.FormatInt(time.Now().Unix(), 10)
			}
			cat.Slug = slug
		}
		cat.Name = name
		cat.Description = desc
		cat.Icon = icon
		cat.ColorClass = colorClass
		if err := config.DB.Save(&cat).Error; err != nil {
			http.Redirect(w, r, config.Path("/admin/knowledge-base/categories/edit/")+strconv.Itoa(id)+"?error=Gagal+menyimpan", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Kategori+berhasil+diperbarui", http.StatusSeeOther)
		return
	}

	errMsg := r.URL.Query().Get("error")
	data := AddBaseData(r, map[string]interface{}{
		"title":         "Edit Kategori KB",
		"page_title":    "Edit Kategori",
		"nav_active":    "admin_kb",
		"template_name": "admin/kb_category_edit",
		"category":      cat,
		"error":         errMsg,
	})
	if user != nil && user.IsStaff && !user.IsSuperAdmin {
		data["nav_active"] = "kb_admin"
		data["template_name"] = "department_kb_category_edit"
		RenderTemplate(w, "department_kb_category_edit", data)
		return
	}
	RenderTemplate(w, "admin/kb_category_edit", data)
}

// DeleteKBCategory menghapus kategori (POST). Artikel di kategori ini tidak dihapus, category_id bisa dibiarkan atau perlu di-handle.
func (h *AdminHandler) DeleteKBCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, config.Path("/admin/knowledge-base"), http.StatusSeeOther)
		return
	}
	id := parseKBIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}
	result := config.DB.Delete(&models.KBCategory{}, id)
	if result.Error != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Gagal+menghapus+kategori", http.StatusSeeOther)
		return
	}
	if result.RowsAffected == 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Kategori+tidak+ditemukan", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Kategori+berhasil+dihapus", http.StatusSeeOther)
}

// EditKBArticle menampilkan form edit (GET) atau menyimpan perubahan (POST). Path: .../articles/edit/<id>
func (h *AdminHandler) EditKBArticle(w http.ResponseWriter, r *http.Request) {
	id := parseKBIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=ID+artikel+tidak+valid", http.StatusSeeOther)
		return
	}
	var art models.KBArticle
	if config.DB.Preload("Category").First(&art, id).Error != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Artikel+tidak+ditemukan", http.StatusSeeOther)
		return
	}
	userVal := GetUserFromContext(r)
	var user *models.User
	if userVal != nil {
		if u, ok := userVal.(*models.User); ok {
			user = u
		}
	}

	if r.Method == http.MethodPost {
		title := strings.TrimSpace(r.FormValue("title"))
		content := r.FormValue("content")
		categoryIDStr := r.FormValue("category_id")
		readTimeStr := strings.TrimSpace(r.FormValue("read_time_minutes"))
		if title == "" || categoryIDStr == "" {
			http.Redirect(w, r, config.Path("/admin/knowledge-base/articles/edit/")+strconv.Itoa(id)+"?error=Judul+dan+kategori+wajib", http.StatusSeeOther)
			return
		}
		existingImagePaths := make(map[int]string)
		if art.Sections != "" {
			var existing []models.KBArticleSection
			_ = json.Unmarshal([]byte(art.Sections), &existing)
			for i, s := range existing {
				if s.ImagePath != "" {
					existingImagePaths[i] = s.ImagePath
				}
			}
		}
		sectionsJSON, _ := parseArticleSectionsFromRequest(r, art.ID, existingImagePaths)
		catID, _ := strconv.Atoi(categoryIDStr)
		readTime, _ := strconv.Atoi(readTimeStr)
		slug := slugify(title)
		if slug != art.Slug {
			var existing models.KBArticle
			if config.DB.Where("slug = ? AND id != ?", slug, id).First(&existing).Error == nil {
				slug = slug + "-" + strconv.FormatInt(time.Now().Unix(), 10)
			}
			art.Slug = slug
		}
		art.CategoryID = uint(catID)
		art.Title = title
		art.Content = content
		art.Sections = sectionsJSON
		art.ReadTimeMinutes = readTime
		if err := config.DB.Save(&art).Error; err != nil {
			http.Redirect(w, r, config.Path("/admin/knowledge-base/articles/edit/")+strconv.Itoa(id)+"?error=Gagal+menyimpan", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Artikel+berhasil+diperbarui", http.StatusSeeOther)
		return
	}

	var categories []models.KBCategory
	config.DB.Order("sort_order ASC, name ASC").Find(&categories)
	var articleSections []models.KBArticleSection
	if art.Sections != "" {
		_ = json.Unmarshal([]byte(art.Sections), &articleSections)
	}
	errMsg := r.URL.Query().Get("error")
	data := AddBaseData(r, map[string]interface{}{
		"title":                     "Edit Artikel KB",
		"page_title":                "Edit Artikel",
		"nav_active":                "admin_kb",
		"template_name":             "admin/kb_article_edit",
		"article":                   art,
		"categories":                categories,
		"article_sections":          articleSections,
		"article_sections_next_index": len(articleSections),
		"error":                     errMsg,
	})
	if user != nil && user.IsStaff && !user.IsSuperAdmin {
		data["nav_active"] = "kb_admin"
		data["template_name"] = "department_kb_article_edit"
		RenderTemplate(w, "department_kb_article_edit", data)
		return
	}
	RenderTemplate(w, "admin/kb_article_edit", data)
}

// DeleteKBArticle menghapus artikel (POST).
func (h *AdminHandler) DeleteKBArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, config.Path("/admin/knowledge-base"), http.StatusSeeOther)
		return
	}
	id := parseKBIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}
	result := config.DB.Delete(&models.KBArticle{}, id)
	if result.Error != nil {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Gagal+menghapus+artikel", http.StatusSeeOther)
		return
	}
	if result.RowsAffected == 0 {
		http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?error=Artikel+tidak+ditemukan", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, config.Path("/admin/knowledge-base")+"?success=Artikel+berhasil+dihapus", http.StatusSeeOther)
}

// SearchAdmin handles the natural language search via Google AI
func (h *AdminHandler) SearchAdmin(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, config.Path("admin/dashboard"), http.StatusSeeOther)
		return
	}

	filters, err := h.aiService.TranslateQueryToFilters(r.Context(), query)
	if err != nil {
		log.Printf("Error translating query via AI: %v", err)
		filters = services.AIFilters{Keyword: query}
	}

	tickets, err := h.adminSearch.SearchTickets(filters)
	if err != nil {
		log.Printf("Error searching tickets: %v", err)
	}

	// Step 2: Ask AI to analyze the results and answer the question
	aiAnswer := ""
	if len(tickets) > 0 {
		answer, err := h.aiService.AnalyzeTickets(r.Context(), query, tickets)
		if err != nil {
			log.Printf("Error analyzing tickets via AI: %v", err)
		} else {
			aiAnswer = answer
		}
	}

	data := utils.AddBaseData(r, map[string]interface{}{
		"title":         "Hasil Pencarian: " + query,
		"page_title":    "Smart Search Results",
		"page_subtitle": "Hasil pencarian AI untuk: \"" + query + "\"",
		"template_name": "admin/search_results",
		"nav_active":    "admin_search",
		"query":         query,
		"filters":       filters,
		"tickets":       tickets,
		"ai_answer":     aiAnswer,
	})
	utils.RenderTemplate(w, "admin_search_results", data)
}

// --- Centralized Admin Management for Companies (Multi-PT) ---

// parseIDFromPath extracts the numeric ID from the last segment of the URL path.
// It safely strips query strings and handles both stripped ("/companies/edit/1") and unstripped ("/admin/companies/edit/1") paths.
func parseIDFromPath(path string) int {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return 0
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0
	}
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// ListCompanies menampilkan daftar perusahaan dengan relasi departemen.
func (h *AdminHandler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var companies []models.Company
	if err := config.DB.Preload("Departments").Order("id ASC").Find(&companies).Error; err != nil {
		log.Printf("[Security][Admin] Failed to load companies: %v", err)
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kelola Perusahaan - Admin Panel",
		"page_title":    "Manajemen Perusahaan",
		"page_subtitle": "Kelola entitas perusahaan (Multi-PT) dan departemen di dalamnya",
		"nav_active":    "admin_companies",
		"template_name": "admin/companies_list",
		"companies":     companies,
		"error":         r.URL.Query().Get("error"),
		"success":       r.URL.Query().Get("success"),
	})

	RenderTemplate(w, "admin/companies_list", data)
}

// CreateCompanyForm menampilkan form tambah perusahaan (GET) atau menyimpan (POST).
func (h *AdminHandler) CreateCompanyForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := AddBaseData(r, map[string]interface{}{
			"title":         "Tambah Perusahaan Baru",
			"page_title":    "Tambah Perusahaan",
			"page_subtitle": "Tambah unit tenant perusahaan (Multi-PT)",
			"nav_active":    "admin_companies",
			"template_name": "admin/company_form",
			"error":         r.URL.Query().Get("error"),
			"success":       r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/company_form", data)
		return
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
		address := strings.TrimSpace(r.FormValue("address"))
		phone := strings.TrimSpace(r.FormValue("phone"))

		if name == "" {
			http.Redirect(w, r, config.Path("/admin/companies/create")+"?error="+url.QueryEscape("Nama perusahaan wajib diisi"), http.StatusSeeOther)
			return
		}

		if code == "" {
			http.Redirect(w, r, config.Path("/admin/companies/create")+"?error="+url.QueryEscape("Kode perusahaan wajib diisi"), http.StatusSeeOther)
			return
		}

		if len(code) > 20 {
			http.Redirect(w, r, config.Path("/admin/companies/create")+"?error="+url.QueryEscape("Kode perusahaan maksimal 20 karakter"), http.StatusSeeOther)
			return
		}

		var count int64
		if err := config.DB.Model(&models.Company{}).Where("LOWER(code) = LOWER(?)", code).Count(&count).Error; err != nil {
			log.Printf("[Security][Admin] Error checking company code uniqueness: %v", err)
		}
		if count > 0 {
			http.Redirect(w, r, config.Path("/admin/companies/create")+"?error="+url.QueryEscape("Kode perusahaan sudah digunakan"), http.StatusSeeOther)
			return
		}

		isActive := true
		if val := strings.TrimSpace(r.FormValue("is_active")); val != "" {
			if val == "false" || val == "0" {
				isActive = false
			}
		}

		newComp := models.Company{
			Name:     name,
			Code:     code,
			Address:  address,
			Phone:    phone,
			IsActive: isActive,
		}

		if err := config.DB.Create(&newComp).Error; err != nil {
			log.Printf("[Security][Admin] Failed to create company: %v", err)
			http.Redirect(w, r, config.Path("/admin/companies/create")+"?error="+url.QueryEscape("Gagal menyimpan perusahaan"), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, config.Path("/admin/companies")+"?success="+url.QueryEscape("Perusahaan berhasil dibuat"), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// EditCompanyForm menampilkan form edit perusahaan (GET) atau menyimpan pembaruan (POST).
func (h *AdminHandler) EditCompanyForm(w http.ResponseWriter, r *http.Request) {
	id := parseIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/companies")+"?error="+url.QueryEscape("ID perusahaan tidak valid"), http.StatusSeeOther)
		return
	}

	var comp models.Company
	if err := config.DB.First(&comp, id).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/companies")+"?error="+url.QueryEscape("Perusahaan tidak ditemukan"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		data := AddBaseData(r, map[string]interface{}{
			"title":         "Edit Perusahaan - " + comp.Name,
			"page_title":    "Edit Perusahaan",
			"page_subtitle": "Perbarui informasi perusahaan",
			"nav_active":    "admin_companies",
			"template_name": "admin/company_edit",
			"company":       comp,
			"error":         r.URL.Query().Get("error"),
			"success":       r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/company_edit", data)
		return
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
		address := strings.TrimSpace(r.FormValue("address"))
		phone := strings.TrimSpace(r.FormValue("phone"))

		editURL := fmt.Sprintf("%s?error=", config.Path(fmt.Sprintf("/admin/companies/edit/%d", id)))

		if name == "" {
			http.Redirect(w, r, editURL+url.QueryEscape("Nama perusahaan wajib diisi"), http.StatusSeeOther)
			return
		}

		if code == "" {
			http.Redirect(w, r, editURL+url.QueryEscape("Kode perusahaan wajib diisi"), http.StatusSeeOther)
			return
		}

		if len(code) > 20 {
			http.Redirect(w, r, editURL+url.QueryEscape("Kode perusahaan maksimal 20 karakter"), http.StatusSeeOther)
			return
		}

		var count int64
		if err := config.DB.Model(&models.Company{}).Where("LOWER(code) = LOWER(?) AND id != ?", code, id).Count(&count).Error; err != nil {
			log.Printf("[Security][Admin] Error checking company code uniqueness on edit: %v", err)
		}
		if count > 0 {
			http.Redirect(w, r, editURL+url.QueryEscape("Kode perusahaan sudah digunakan"), http.StatusSeeOther)
			return
		}

		comp.Name = name
		comp.Code = code
		comp.Address = address
		comp.Phone = phone

		if val := strings.TrimSpace(r.FormValue("is_active")); val != "" {
			if val == "false" || val == "0" {
				comp.IsActive = false
			} else if val == "true" || val == "1" || val == "on" {
				comp.IsActive = true
			}
		}

		if err := config.DB.Save(&comp).Error; err != nil {
			log.Printf("[Security][Admin] Failed to update company: %v", err)
			http.Redirect(w, r, editURL+url.QueryEscape("Gagal memperbarui perusahaan"), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, config.Path("/admin/companies")+"?success="+url.QueryEscape("Perusahaan berhasil diperbarui"), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// ToggleCompanyStatus mengaktifkan atau menonaktifkan status perusahaan (mendukung GET dan POST).
func (h *AdminHandler) ToggleCompanyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	id := parseIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/companies")+"?error="+url.QueryEscape("ID perusahaan tidak valid"), http.StatusSeeOther)
		return
	}

	var comp models.Company
	if err := config.DB.First(&comp, id).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/companies")+"?error="+url.QueryEscape("Perusahaan tidak ditemukan"), http.StatusSeeOther)
		return
	}

	comp.IsActive = !comp.IsActive
	if err := config.DB.Save(&comp).Error; err != nil {
		log.Printf("[Security][Admin] Failed to toggle company status: %v", err)
		http.Redirect(w, r, config.Path("/admin/companies")+"?error="+url.QueryEscape("Gagal mengubah status perusahaan"), http.StatusSeeOther)
		return
	}

	msg := "Perusahaan berhasil dinonaktifkan"
	if comp.IsActive {
		msg = "Perusahaan berhasil diaktifkan"
	}
	http.Redirect(w, r, config.Path("/admin/companies")+"?success="+url.QueryEscape(msg), http.StatusSeeOther)
}

// --- SLA Policy Admin Handlers ---

func parseUintPtr(s string) *uint {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return nil
	}
	val, err := strconv.ParseUint(s, 10, 32)
	if err != nil || val == 0 {
		return nil
	}
	u := uint(val)
	return &u
}

func parseIntValue(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	val, err := strconv.Atoi(s)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

// ListSLAPolicies menampilkan daftar kebijakan SLA per PT dan Departemen.
func (h *AdminHandler) ListSLAPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var policies []models.SLAPolicy
	if err := config.DB.Preload("Company").Preload("Department").
		Order("is_default DESC, company_id ASC, department_id ASC, id ASC").
		Find(&policies).Error; err != nil {
		log.Printf("[Admin] Failed to load SLA policies: %v", err)
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kebijakan SLA - Admin Panel",
		"page_title":    "Kebijakan SLA",
		"page_subtitle": "Kelola target waktu respon pertama dan resolusi per prioritas, PT, dan departemen",
		"nav_active":    "admin_sla_policies",
		"template_name": "admin/sla_policies_list",
		"policies":      policies,
		"error":         r.URL.Query().Get("error"),
		"success":       r.URL.Query().Get("success"),
	})

	RenderTemplate(w, "admin/sla_policies_list", data)
}

// CreateSLAPolicyForm menampilkan form pembuatan kebijakan SLA (GET) atau menyimpannya (POST).
func (h *AdminHandler) CreateSLAPolicyForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var companies []models.Company
		config.DB.Where("is_active = ?", true).Preload("Departments").Order("name ASC").Find(&companies)

		var departments []models.Department
		config.DB.Preload("Company").Order("name ASC").Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":             "Tambah Kebijakan SLA Baru",
			"page_title":        "Tambah Kebijakan SLA",
			"page_subtitle":     "Konfigurasi aturan target First Response dan Resolusi",
			"nav_active":        "admin_sla_policies",
			"template_name":     "admin/sla_policy_form",
			"is_edit":           false,
			"companies":         companies,
			"departments":       departments,
			"resp_high_hours":   1,
			"resp_high_minutes": 0,
			"resp_med_hours":    4,
			"resp_med_minutes":  0,
			"resp_low_hours":    8,
			"resp_low_minutes":  0,
			"resol_high_days":   0,
			"resol_high_hours":  4,
			"resol_med_days":    1,
			"resol_med_hours":   0,
			"resol_low_days":    3,
			"resol_low_hours":   0,
			"error":             r.URL.Query().Get("error"),
			"success":           r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/sla_policy_form", data)
		return
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		createURL := config.Path("/admin/sla-policies/create") + "?error="

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Redirect(w, r, createURL+url.QueryEscape("Nama kebijakan SLA wajib diisi"), http.StatusSeeOther)
			return
		}

		description := strings.TrimSpace(r.FormValue("description"))
		companyID := parseUintPtr(r.FormValue("company_id"))
		departmentID := parseUintPtr(r.FormValue("department_id"))

		isDefault := false
		if val := strings.TrimSpace(r.FormValue("is_default")); val == "true" || val == "1" || val == "on" {
			isDefault = true
			companyID = nil
			departmentID = nil
		}

		isActive := true
		if val := strings.TrimSpace(r.FormValue("is_active")); val == "false" || val == "0" {
			isActive = false
		}

		// Calculate dual-unit response durations
		respHigh := parseIntValue(r.FormValue("resp_high_hours"))*60 + parseIntValue(r.FormValue("resp_high_minutes"))
		respMed := parseIntValue(r.FormValue("resp_med_hours"))*60 + parseIntValue(r.FormValue("resp_med_minutes"))
		respLow := parseIntValue(r.FormValue("resp_low_hours"))*60 + parseIntValue(r.FormValue("resp_low_minutes"))

		// Calculate dual-unit resolution durations
		resolHigh := parseIntValue(r.FormValue("resol_high_days"))*24 + parseIntValue(r.FormValue("resol_high_hours"))
		resolMed := parseIntValue(r.FormValue("resol_med_days"))*24 + parseIntValue(r.FormValue("resol_med_hours"))
		resolLow := parseIntValue(r.FormValue("resol_low_days"))*24 + parseIntValue(r.FormValue("resol_low_hours"))

		if respHigh <= 0 || respMed <= 0 || respLow <= 0 {
			http.Redirect(w, r, createURL+url.QueryEscape("Target waktu respon pertama minimal 1 menit untuk setiap prioritas"), http.StatusSeeOther)
			return
		}

		if resolHigh <= 0 || resolMed <= 0 || resolLow <= 0 {
			http.Redirect(w, r, createURL+url.QueryEscape("Target waktu resolusi minimal 1 jam untuk setiap prioritas"), http.StatusSeeOther)
			return
		}

		// Validate department belongs to company if specified
		if departmentID != nil {
			var dept models.Department
			if err := config.DB.First(&dept, *departmentID).Error; err != nil {
				http.Redirect(w, r, createURL+url.QueryEscape("Departemen tidak ditemukan"), http.StatusSeeOther)
				return
			}
			if companyID != nil && (dept.CompanyID == nil || *dept.CompanyID != *companyID) {
				http.Redirect(w, r, createURL+url.QueryEscape("Departemen tidak terdaftar pada perusahaan yang dipilih"), http.StatusSeeOther)
				return
			}
			if companyID == nil && dept.CompanyID != nil {
				companyID = dept.CompanyID
			}
		}

		// Scope conflict check
		var dupCount int64
		dupQ := config.DB.Model(&models.SLAPolicy{}).Where("is_active = ?", true)
		if isDefault {
			dupQ = dupQ.Where("is_default = true")
		} else if departmentID != nil {
			dupQ = dupQ.Where("department_id = ?", *departmentID)
		} else if companyID != nil {
			dupQ = dupQ.Where("company_id = ? AND (department_id IS NULL OR department_id = 0)", *companyID)
		}
		if err := dupQ.Count(&dupCount).Error; err == nil && dupCount > 0 && isActive {
			http.Redirect(w, r, createURL+url.QueryEscape("Kebijakan SLA aktif untuk cakupan ini sudah ada. Nonaktifkan kebijakan lama terlebih dahulu."), http.StatusSeeOther)
			return
		}

		policy := models.SLAPolicy{
			Name:                    name,
			Description:             description,
			CompanyID:               companyID,
			DepartmentID:            departmentID,
			IsActive:                isActive,
			IsDefault:               isDefault,
			ResponseTimeHighMinutes: respHigh,
			ResponseTimeMedMinutes:  respMed,
			ResponseTimeLowMinutes:  respLow,
			ResolutionTimeHighHours: resolHigh,
			ResolutionTimeMedHours:  resolMed,
			ResolutionTimeLowHours:  resolLow,
		}

		if err := config.DB.Create(&policy).Error; err != nil {
			log.Printf("[Admin] Failed to create SLA policy: %v", err)
			http.Redirect(w, r, createURL+url.QueryEscape("Gagal menyimpan kebijakan SLA"), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?success="+url.QueryEscape("Kebijakan SLA berhasil dibuat"), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// EditSLAPolicyForm menampilkan form edit kebijakan SLA (GET) atau memperbaruinya (POST).
func (h *AdminHandler) EditSLAPolicyForm(w http.ResponseWriter, r *http.Request) {
	id := parseIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?error="+url.QueryEscape("ID kebijakan SLA tidak valid"), http.StatusSeeOther)
		return
	}

	var policy models.SLAPolicy
	if err := config.DB.Preload("Company").Preload("Department").First(&policy, id).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?error="+url.QueryEscape("Kebijakan SLA tidak ditemukan"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		var companies []models.Company
		config.DB.Where("is_active = ?", true).Preload("Departments").Order("name ASC").Find(&companies)

		var departments []models.Department
		config.DB.Preload("Company").Order("name ASC").Find(&departments)

		data := AddBaseData(r, map[string]interface{}{
			"title":             "Edit Kebijakan SLA - " + policy.Name,
			"page_title":        "Edit Kebijakan SLA",
			"page_subtitle":     "Perbarui target waktu respon dan resolusi",
			"nav_active":        "admin_sla_policies",
			"template_name":     "admin/sla_policy_form",
			"is_edit":           true,
			"policy":            policy,
			"companies":         companies,
			"departments":       departments,
			"resp_high_hours":   policy.ResponseTimeHighMinutes / 60,
			"resp_high_minutes": policy.ResponseTimeHighMinutes % 60,
			"resp_med_hours":    policy.ResponseTimeMedMinutes / 60,
			"resp_med_minutes":  policy.ResponseTimeMedMinutes % 60,
			"resp_low_hours":    policy.ResponseTimeLowMinutes / 60,
			"resp_low_minutes":  policy.ResponseTimeLowMinutes % 60,
			"resol_high_days":   policy.ResolutionTimeHighHours / 24,
			"resol_high_hours":  policy.ResolutionTimeHighHours % 24,
			"resol_med_days":    policy.ResolutionTimeMedHours / 24,
			"resol_med_hours":   policy.ResolutionTimeMedHours % 24,
			"resol_low_days":    policy.ResolutionTimeLowHours / 24,
			"resol_low_hours":   policy.ResolutionTimeLowHours % 24,
			"error":             r.URL.Query().Get("error"),
			"success":           r.URL.Query().Get("success"),
		})
		RenderTemplate(w, "admin/sla_policy_form", data)
		return
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		editURL := fmt.Sprintf("%s?error=", config.Path(fmt.Sprintf("/admin/sla-policies/edit/%d", id)))

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Redirect(w, r, editURL+url.QueryEscape("Nama kebijakan SLA wajib diisi"), http.StatusSeeOther)
			return
		}

		description := strings.TrimSpace(r.FormValue("description"))
		companyID := parseUintPtr(r.FormValue("company_id"))
		departmentID := parseUintPtr(r.FormValue("department_id"))

		isDefault := false
		if val := strings.TrimSpace(r.FormValue("is_default")); val == "true" || val == "1" || val == "on" {
			isDefault = true
			companyID = nil
			departmentID = nil
		}

		isActive := true
		if val := strings.TrimSpace(r.FormValue("is_active")); val == "false" || val == "0" {
			isActive = false
		}

		respHigh := parseIntValue(r.FormValue("resp_high_hours"))*60 + parseIntValue(r.FormValue("resp_high_minutes"))
		respMed := parseIntValue(r.FormValue("resp_med_hours"))*60 + parseIntValue(r.FormValue("resp_med_minutes"))
		respLow := parseIntValue(r.FormValue("resp_low_hours"))*60 + parseIntValue(r.FormValue("resp_low_minutes"))

		resolHigh := parseIntValue(r.FormValue("resol_high_days"))*24 + parseIntValue(r.FormValue("resol_high_hours"))
		resolMed := parseIntValue(r.FormValue("resol_med_days"))*24 + parseIntValue(r.FormValue("resol_med_hours"))
		resolLow := parseIntValue(r.FormValue("resol_low_days"))*24 + parseIntValue(r.FormValue("resol_low_hours"))

		if respHigh <= 0 || respMed <= 0 || respLow <= 0 {
			http.Redirect(w, r, editURL+url.QueryEscape("Target waktu respon pertama minimal 1 menit untuk setiap prioritas"), http.StatusSeeOther)
			return
		}

		if resolHigh <= 0 || resolMed <= 0 || resolLow <= 0 {
			http.Redirect(w, r, editURL+url.QueryEscape("Target waktu resolusi minimal 1 jam untuk setiap prioritas"), http.StatusSeeOther)
			return
		}

		if departmentID != nil {
			var dept models.Department
			if err := config.DB.First(&dept, *departmentID).Error; err != nil {
				http.Redirect(w, r, editURL+url.QueryEscape("Departemen tidak ditemukan"), http.StatusSeeOther)
				return
			}
			if companyID != nil && (dept.CompanyID == nil || *dept.CompanyID != *companyID) {
				http.Redirect(w, r, editURL+url.QueryEscape("Departemen tidak terdaftar pada perusahaan yang dipilih"), http.StatusSeeOther)
				return
			}
			if companyID == nil && dept.CompanyID != nil {
				companyID = dept.CompanyID
			}
		}

		// Scope conflict check (excluding current record ID)
		var dupCount int64
		dupQ := config.DB.Model(&models.SLAPolicy{}).Where("is_active = ? AND id != ?", true, id)
		if isDefault {
			dupQ = dupQ.Where("is_default = true")
		} else if departmentID != nil {
			dupQ = dupQ.Where("department_id = ?", *departmentID)
		} else if companyID != nil {
			dupQ = dupQ.Where("company_id = ? AND (department_id IS NULL OR department_id = 0)", *companyID)
		}
		if err := dupQ.Count(&dupCount).Error; err == nil && dupCount > 0 && isActive {
			http.Redirect(w, r, editURL+url.QueryEscape("Kebijakan SLA aktif lain untuk cakupan ini sudah ada."), http.StatusSeeOther)
			return
		}

		policy.Name = name
		policy.Description = description
		policy.CompanyID = companyID
		policy.DepartmentID = departmentID
		policy.IsActive = isActive
		policy.IsDefault = isDefault
		policy.ResponseTimeHighMinutes = respHigh
		policy.ResponseTimeMedMinutes = respMed
		policy.ResponseTimeLowMinutes = respLow
		policy.ResolutionTimeHighHours = resolHigh
		policy.ResolutionTimeMedHours = resolMed
		policy.ResolutionTimeLowHours = resolLow

		if err := config.DB.Save(&policy).Error; err != nil {
			log.Printf("[Admin] Failed to update SLA policy: %v", err)
			http.Redirect(w, r, editURL+url.QueryEscape("Gagal memperbarui kebijakan SLA"), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?success="+url.QueryEscape("Kebijakan SLA berhasil diperbarui"), http.StatusSeeOther)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// ToggleSLAPolicyStatus mengaktifkan atau menonaktifkan status kebijakan SLA.
func (h *AdminHandler) ToggleSLAPolicyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	id := parseIDFromPath(r.URL.Path)
	if id <= 0 {
		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?error="+url.QueryEscape("ID kebijakan SLA tidak valid"), http.StatusSeeOther)
		return
	}

	var policy models.SLAPolicy
	if err := config.DB.First(&policy, id).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?error="+url.QueryEscape("Kebijakan SLA tidak ditemukan"), http.StatusSeeOther)
		return
	}

	policy.IsActive = !policy.IsActive
	if err := config.DB.Save(&policy).Error; err != nil {
		log.Printf("[Admin] Failed to toggle SLA policy status: %v", err)
		http.Redirect(w, r, config.Path("/admin/sla-policies")+"?error="+url.QueryEscape("Gagal mengubah status kebijakan SLA"), http.StatusSeeOther)
		return
	}

	msg := "Kebijakan SLA berhasil dinonaktifkan"
	if policy.IsActive {
		msg = "Kebijakan SLA berhasil diaktifkan"
	}
	http.Redirect(w, r, config.Path("/admin/sla-policies")+"?success="+url.QueryEscape(msg), http.StatusSeeOther)
}

// ShowReports menampilkan halaman laporan kinerja bulanan admin.
func (h *AdminHandler) ShowReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.adminReportService == nil {
		h.adminReportService = services.NewAdminReportService()
	}

	now := time.Now()
	month := int(now.Month())
	year := now.Year()

	periodQuery := strings.TrimSpace(r.URL.Query().Get("period"))
	if periodQuery != "" {
		if t, err := time.Parse("2006-01", periodQuery); err == nil {
			month = int(t.Month())
			year = t.Year()
		}
	} else {
		if m, err := strconv.Atoi(r.URL.Query().Get("month")); err == nil && m >= 1 && m <= 12 {
			month = m
		}
		if y, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && y >= 2000 {
			year = y
		}
	}

	var companyIDPtr *uint
	companyIDVal := uint(0)
	if compStr := strings.TrimSpace(r.URL.Query().Get("company_id")); compStr != "" {
		if cid, err := strconv.ParseUint(compStr, 10, 64); err == nil && cid > 0 {
			uCID := uint(cid)
			companyIDPtr = &uCID
			companyIDVal = uCID
		}
	}

	var adminUserID uint
	var adminEmail string
	if u := GetUserFromContext(r); u != nil {
		if userObj, ok := u.(*models.User); ok && userObj != nil {
			adminUserID = userObj.ID
			if userObj.Email != nil {
				adminEmail = *userObj.Email
			}
		}
	}

	logging.AdminReports.Info("Admin performance report requested",
		"admin_user_id", adminUserID,
		"admin_email", adminEmail,
		"raw_period_query", periodQuery,
		"filter_month", month,
		"filter_year", year,
		"filter_company_id", companyIDVal,
		"remote_ip", r.RemoteAddr,
	)

	filter := services.MonthlyReportFilter{
		Month:     month,
		Year:      year,
		CompanyID: companyIDPtr,
	}

	reportData, err := h.adminReportService.GetMonthlyCompanyReport(filter)
	if err != nil {
		logging.AdminReports.Error("Failed to generate monthly report",
			"error", err.Error(),
			"filter_month", month,
			"filter_year", year,
			"filter_company_id", companyIDVal,
		)
		log.Printf("[Admin] Failed to generate monthly report: %v", err)
		http.Error(w, "Gagal memuat laporan kinerja", http.StatusInternalServerError)
		return
	}

	logging.AdminReports.Info("Admin performance report generated successfully",
		"period_label", reportData.PeriodLabel,
		"companies_count", len(reportData.CompanyList),
		"grand_total_tickets", reportData.GrandTotal.TotalTickets,
		"open_tickets", reportData.GrandTotal.OpenTickets,
		"closed_tickets", reportData.GrandTotal.ClosedTickets,
		"first_response_breached", reportData.GrandTotal.FirstResponseBreached,
		"resolution_breached", reportData.GrandTotal.ResolutionBreached,
		"sla_met_rate", reportData.GrandTotal.OverallSLAMetRate,
		"rated_count", reportData.GrandTotal.TotalRatedCount,
		"avg_rating", reportData.GrandTotal.OverallAvgRating,
	)

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Laporan Kinerja Penanganan Tiket — Ticketing",
		"page_title":    "Laporan Kinerja",
		"page_subtitle": "Rekapitulasi Bulanan Kinerja Tiket per Perusahaan & Departemen",
		"nav_active":    "admin_reports",
		"template_name": "admin/reports",
		"report":        reportData,
	})

	RenderTemplate(w, "admin/reports", data)
}

