package handlers

import (
	"encoding/json"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
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
	cfg              *config.Config
	adminDashService *services.AdminDashboardService
}

func NewAdminHandler(cfg *config.Config, adminDashService *services.AdminDashboardService) *AdminHandler {
	return &AdminHandler{cfg: cfg, adminDashService: adminDashService}
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

// ListUsers menampilkan daftar user dengan filter role (user/staff) untuk admin.
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
		"template_name": "admin/users_list",
		"users":         users,
		"filter":        filter,
		"user":          user,
	})

	RenderTemplate(w, "admin/users_list", data)
}

// CreateUserForm menampilkan form tambah user (GET) atau menyimpan user baru (POST).
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
			if departmentID == nil {
				http.Error(w, "Staff wajib memiliki departemen", http.StatusBadRequest)
				return
			}
		} else if role == "admin" {
			newUser.IsStaff = true
			newUser.IsSuperAdmin = true
			newUser.DepartmentID = nil
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

// ToggleUserStatus mengaktifkan/nonaktifkan user (untuk admin).
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

// ToggleStaffRole mengubah user jadi staff (pilih dept) atau turunkan jadi user biasa.
func (h *AdminHandler) ToggleStaffRole(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/users/staff/")
	userID, _ := strconv.Atoi(path)

	var targetUser models.User
	if err := config.DB.Preload("Department").First(&targetUser, userID).Error; err != nil {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	if targetUser.IsSuperAdmin {
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet && targetUser.IsStaff {
		targetUser.IsStaff = false
		targetUser.DepartmentID = nil
		config.DB.Save(&targetUser)
		http.Redirect(w, r, config.Path("/admin/users"), http.StatusSeeOther)
		return
	}

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

// ListDepartments menampilkan daftar departemen dengan statistik rating dan tiket selesai.
func (h *AdminHandler) ListDepartments(w http.ResponseWriter, r *http.Request) {
	var departments []models.Department
	config.DB.Preload("Tickets").Find(&departments)

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
	})

	RenderTemplate(w, "admin/departments_list", data)
}

// CreateDepartmentForm menampilkan form tambah departemen (GET) atau menyimpan (POST).
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
	if user != nil && user.IsStaff {
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
	if user != nil && user.IsStaff {
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
	if user != nil && user.IsStaff {
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

func saveUploadedFile(fh *multipart.FileHeader, dstPath string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
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
	if user != nil && user.IsStaff {
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
	if user != nil && user.IsStaff {
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