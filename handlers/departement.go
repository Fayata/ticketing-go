package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"

	"gorm.io/gorm"
)

type DepartmentHandler struct {
	cfg                   *config.Config
	emailService          *utils.EmailService
	staffDashboardService *services.StaffDashboardService
	wsHub                 *services.WSHub
}

func NewDepartmentHandler(cfg *config.Config, emailService *utils.EmailService, staffDashboardService *services.StaffDashboardService, wsHub ...*services.WSHub) *DepartmentHandler {
	var hub *services.WSHub
	if len(wsHub) > 0 {
		hub = wsHub[0]
	}
	return &DepartmentHandler{
		cfg:                   cfg,
		emailService:          emailService,
		staffDashboardService: staffDashboardService,
		wsHub:                 hub,
	}
}

func (h *DepartmentHandler) SetWSHub(hub *services.WSHub) {
	h.wsHub = hub
}

// addDepartmentData menambah data dasar (user, nav) untuk semua halaman staff/departemen.
func (h *DepartmentHandler) addDepartmentData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	baseData := AddBaseData(r, data)
	baseData["is_department_page"] = true
	return baseData
}

type MonthlyStat struct {
	MonthLabel string
	Count      int
	Height     string
}

// ShowDashboard menampilkan halaman dashboard staff: KPI, grafik, tiket saya, pool, belum di-rate.
func (h *DepartmentHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	if user.IsSuperAdmin {
		http.Redirect(w, r, config.Path("/admin/dashboard"), http.StatusSeeOther)
		return
	}

	var dbUser models.User
	if err := config.DB.Select("id", "department_id").First(&dbUser, user.ID).Error; err != nil {
		http.Error(w, "User tidak ditemukan.", http.StatusInternalServerError)
		return
	}
	if dbUser.DepartmentID == nil || *dbUser.DepartmentID == 0 {
		http.Redirect(w, r, config.Path("/departement/all-tickets")+"?error=Akun+staff+belum+memiliki+departemen", http.StatusSeeOther)
		return
	}
	deptID := *dbUser.DepartmentID

	if h.staffDashboardService == nil {
		http.Error(w, "Dashboard service not configured", http.StatusInternalServerError)
		return
	}

	dash, err := h.staffDashboardService.GetStaffDashboardData(user.ID, deptID)
	if err != nil {
		http.Error(w, "Gagal memuat dashboard", http.StatusInternalServerError)
		return
	}

	trendJSON, _ := json.Marshal(dash.TrendData)
	monthlyJSON, _ := json.Marshal(dash.MonthlyData)
	donutJSON, _ := json.Marshal(dash.DonutData)
	trendSafe := template.JS(escapeJSONForScript(trendJSON))
	monthlySafe := template.JS(escapeJSONForScript(monthlyJSON))
	donutSafe := template.JS(escapeJSONForScript(donutJSON))

	successMsg := r.URL.Query().Get("success")

	kpi := map[string]interface{}{
		"WaitingCount":     dash.WaitingCount,
		"ProgressCount":    dash.ProgressCount,
		"ClosedTodayCount": dash.ClosedTodayCount,
		"ClosedMonthCount": dash.ClosedMonthCount,
		"AvgRating":        dash.AvgRating,
		"RatedCount":       dash.RatedCount,
		"TrendClosedPct":   dash.TrendClosedPct,
		"TrendMonthPct":    dash.TrendMonthPct,
	}

	data := h.addDepartmentData(r, map[string]interface{}{
		"title":              "Dashboard Departemen",
		"page_title":         "Department Area",
		"page_subtitle":      dash.DepartmentName,
		"nav_active":         "dept_dashboard",
		"template_name":      "tickets/department_dashboard",
		"user":               user,
		"dashboard":          dash,
		"kpi":                kpi,
		"success":            successMsg,
		"trend_data_json":    trendSafe,
		"monthly_data_json":  monthlySafe,
		"donut_data_json":    donutSafe,
	})

	RenderTemplate(w, "tickets/department_dashboard", data)
}

// GetLiveDashboardAPI returns JSON of staff dashboard data for real-time auto-refresh without page reload.
func (h *DepartmentHandler) GetLiveDashboardAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := GetUserFromContext(r).(*models.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var dbUser models.User
	if err := config.DB.Select("id", "department_id").First(&dbUser, user.ID).Error; err != nil || dbUser.DepartmentID == nil || *dbUser.DepartmentID == 0 {
		http.Error(w, "User departemen tidak valid", http.StatusBadRequest)
		return
	}
	deptID := *dbUser.DepartmentID

	if h.staffDashboardService == nil {
		http.Error(w, "Dashboard service not configured", http.StatusInternalServerError)
		return
	}

	dash, err := h.staffDashboardService.GetStaffDashboardData(user.ID, deptID)
	if err != nil {
		http.Error(w, "Gagal memuat data", http.StatusInternalServerError)
		return
	}

	type poolTicketJSON struct {
		ID              uint   `json:"id"`
		TicketNumber    string `json:"ticket_number"`
		Title           string `json:"title"`
		Priority        string `json:"priority"`
		PriorityDisplay string `json:"priority_display"`
		DepartmentName  string `json:"department_name"`
		CreatorName     string `json:"creator_name"`
		CreatedAtAgo    string `json:"created_at_ago"`
		SLABadgeClass   string `json:"sla_badge_class"`
		SLABadgeLabel   string `json:"sla_badge_label"`
		SLABadgeDetail  string `json:"sla_badge_detail"`
	}

	poolList := make([]poolTicketJSON, 0, len(dash.TicketPool))
	for _, t := range dash.TicketPool {
		deptName := "—"
		if t.Department != nil {
			deptName = t.Department.Name
		}
		creator := t.CreatedBy.Username
		sla := t.GetSLABadgeInfo()
		poolList = append(poolList, poolTicketJSON{
			ID:              t.ID,
			TicketNumber:    t.GetTicketNumber(),
			Title:           t.Title,
			Priority:        string(t.Priority),
			PriorityDisplay: t.GetPriorityDisplay(),
			DepartmentName:  deptName,
			CreatorName:     creator,
			CreatedAtAgo:    timeSinceShort(t.CreatedAt),
			SLABadgeClass:   sla.Class,
			SLABadgeLabel:   sla.Label,
			SLABadgeDetail:  sla.Detail,
		})
	}

	type breachedTicketJSON struct {
		ID              uint   `json:"id"`
		TicketNumber    string `json:"ticket_number"`
		Title           string `json:"title"`
		Priority        string `json:"priority"`
		PriorityDisplay string `json:"priority_display"`
		OverdueDetail   string `json:"overdue_detail"`
	}

	breachedList := make([]breachedTicketJSON, 0, len(dash.SLABreachedTickets))
	for _, t := range dash.SLABreachedTickets {
		sla := t.GetSLABadgeInfo()
		breachedList = append(breachedList, breachedTicketJSON{
			ID:              t.ID,
			TicketNumber:    t.GetTicketNumber(),
			Title:           t.Title,
			Priority:        string(t.Priority),
			PriorityDisplay: t.GetPriorityDisplay(),
			OverdueDetail:   sla.Detail,
		})
	}

	type myTicketJSON struct {
		ID             uint   `json:"id"`
		TicketNumber   string `json:"ticket_number"`
		Title          string `json:"title"`
		Status         string `json:"status"`
		StatusDisplay  string `json:"status_display"`
		Priority       string `json:"priority"`
		DepartmentName string `json:"department_name"`
		UpdatedAtAgo   string `json:"updated_at_ago"`
	}

	myList := make([]myTicketJSON, 0, len(dash.MyActiveTickets))
	for _, t := range dash.MyActiveTickets {
		deptName := "—"
		if t.Department != nil {
			deptName = t.Department.Name
		}
		myList = append(myList, myTicketJSON{
			ID:             t.ID,
			TicketNumber:   t.GetTicketNumber(),
			Title:          t.Title,
			Status:         string(t.Status),
			StatusDisplay:  t.GetStatusDisplay(),
			Priority:       string(t.Priority),
			DepartmentName: deptName,
			UpdatedAtAgo:   timeSinceShort(t.UpdatedAt),
		})
	}

	type unratedTicketJSON struct {
		ID           uint   `json:"id"`
		TicketNumber string `json:"ticket_number"`
		Title        string `json:"title"`
		UpdatedAt    string `json:"updated_at"`
	}

	unratedList := make([]unratedTicketJSON, 0, len(dash.UnratedTickets))
	for _, t := range dash.UnratedTickets {
		unratedList = append(unratedList, unratedTicketJSON{
			ID:           t.ID,
			TicketNumber: t.GetTicketNumber(),
			Title:        t.Title,
			UpdatedAt:    t.UpdatedAt.Format("02 Jan 2006"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"kpi": map[string]interface{}{
			"WaitingCount":     dash.WaitingCount,
			"ProgressCount":    dash.ProgressCount,
			"ClosedTodayCount": dash.ClosedTodayCount,
			"ClosedMonthCount": dash.ClosedMonthCount,
			"AvgRating":        dash.AvgRating,
			"RatedCount":       dash.RatedCount,
			"TrendClosedPct":   dash.TrendClosedPct,
			"TrendMonthPct":    dash.TrendMonthPct,
		},
		"pool":        poolList,
		"breached":    breachedList,
		"my_active":   myList,
		"unrated":     unratedList,
		"server_time": time.Now().Format(time.RFC3339),
	})
}

// ShowAllTickets menampilkan daftar semua tiket dengan filter status, tab, dan departemen (halaman staff).
// Tab yang didukung: "" / "all" (semua), "sla_breached" (SLA terlewat), "mine" (tiket saya).
func (h *DepartmentHandler) ShowAllTickets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	var staffUser models.User
	config.DB.Select("id", "department_id", "is_super_admin", "is_staff").First(&staffUser, user.ID)

	activeTab := r.URL.Query().Get("tab") // "", "sla_breached", "mine"
	statusFilter := r.URL.Query().Get("status")
	deptFilter := r.URL.Query().Get("department")
	now := time.Now()

	// Base query factory with strict department isolation
	baseQuery := func() *gorm.DB {
		q := config.DB.Preload("Department").Preload("CreatedBy").Preload("AssignedTo").Model(&models.Ticket{})
		if !user.IsSuperAdmin {
			if staffUser.DepartmentID != nil {
				q = q.Where("tickets.department_id = ?", *staffUser.DepartmentID)
			} else {
				q = q.Where("1 = 0")
			}
		} else if deptFilter != "" && deptFilter != "ALL" {
			q = q.Where("tickets.department_id = ?", deptFilter)
		}
		return q
	}

	// ── Tab: SLA Breached ──────────────────────────────────────────────────────
	var slaBreachedTickets []*models.Ticket
	var slaBreachedCount int64
	{
		q := baseQuery().
			Where("first_response_at IS NULL").
			Where("first_response_deadline IS NOT NULL").
			Where("first_response_deadline < ?", now).
			Where("status != ?", models.StatusClosed)
		q.Count(&slaBreachedCount)
		if activeTab == "sla_breached" {
			q.Order("first_response_deadline ASC").Find(&slaBreachedTickets)
		}
	}

	// ── Tab: Mine (tiket yang sedang saya tangani) ────────────────────────────
	var mineTickets []*models.Ticket
	var mineCount int64
	{
		q := baseQuery().Where("assigned_to_id = ?", user.ID).Where("status != ?", models.StatusClosed)
		q.Count(&mineCount)
		if activeTab == "mine" {
			q.Order("created_at DESC").Find(&mineTickets)
		}
	}

	// ── Tab: All (default) ────────────────────────────────────────────────────
	var tickets []*models.Ticket
	if activeTab == "" || activeTab == "all" {
		q := baseQuery()
		if statusFilter != "" && statusFilter != "ALL" {
			q = q.Where("status = ?", statusFilter)
		}
		q.Order("created_at DESC").Find(&tickets)
	}

	// Pilih slice yang ditampilkan berdasarkan tab aktif
	displayTickets := tickets
	switch activeTab {
	case "sla_breached":
		displayTickets = slaBreachedTickets
	case "mine":
		displayTickets = mineTickets
	}

	// Ratings map untuk tiket yang sudah closed
	ticketIDs := make([]uint, 0, len(displayTickets))
	for _, t := range displayTickets {
		if t.Status == models.StatusClosed {
			ticketIDs = append(ticketIDs, t.ID)
		}
	}
	var ratings []models.TicketRating
	ratingsMap := make(map[uint]models.TicketRating)
	if len(ticketIDs) > 0 {
		config.DB.Where("ticket_id IN ?", ticketIDs).Find(&ratings)
		for _, r := range ratings {
			ratingsMap[r.TicketID] = r
		}
	}

	var departments []models.Department
	if !user.IsSuperAdmin && staffUser.DepartmentID != nil {
		config.DB.Where("id = ?", *staffUser.DepartmentID).Find(&departments)
	} else {
		config.DB.Find(&departments)
	}

	data := h.addDepartmentData(r, map[string]interface{}{
		"title":              "Semua Tiket - Department",
		"page_title":         "Semua Tiket",
		"page_subtitle":      "Daftar seluruh tiket yang masuk ke sistem",
		"nav_active":         "dept_all_tickets",
		"template_name":      "tickets/department_all_tickets",
		"user":               user,
		"tickets":            displayTickets,
		"departments":        departments,
		"filter_status":      statusFilter,
		"filter_dept":        deptFilter,
		"ratings_map":        ratingsMap,
		"active_tab":         activeTab,
		"sla_breached_count": slaBreachedCount,
		"mine_count":         mineCount,
	})

	RenderTemplate(w, "tickets/department_all_tickets", data)
}

// HandleTicketDetail mengarahkan GET ke detail tiket, POST ke balas tiket.
func (h *DepartmentHandler) HandleTicketDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowTicketDetail(w, r)
	} else if r.Method == http.MethodPost {
		h.DepartmentReply(w, r)
	}
}

// ShowTicketDetail menampilkan halaman detail tiket untuk staff (balas, lepas, tutup).
func (h *DepartmentHandler) ShowTicketDetail(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path)
	if ticketID <= 0 {
		log.Printf("[Staff][TicketDetail] Gagal parse ticket ID dari path: %s", r.URL.Path)
		http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").Preload("Department").Preload("Replies.User").Preload("Replies.Attachments").Preload("Attachments").Preload("AssignedTo").
		Preload("PriorityHistories", func(db *gorm.DB) *gorm.DB { return db.Order("created_at DESC") }).
		Preload("PriorityHistories.ChangedBy").
		Preload("Company").
		First(&ticket, ticketID).Error; err != nil {
		log.Printf("[Staff][TicketDetail] Tiket ID %d tidak ditemukan: %v", ticketID, err)
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tiket+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	// Validasi kepemilikan dan department: Staff hanya boleh akses tiket departemennya sendiri
	var staffUser models.User
	config.DB.Select("id", "department_id", "is_super_admin").First(&staffUser, user.ID)
	if !user.IsSuperAdmin {
		if staffUser.DepartmentID == nil || ticket.DepartmentID == nil || *staffUser.DepartmentID != *ticket.DepartmentID {
			http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Akses+ditolak.+Tiket+berada+di+luar+departemen+Anda", http.StatusSeeOther)
			return
		}
	}

	isOwner := false
	if ticket.AssignedToID != nil && *ticket.AssignedToID == user.ID {
		isOwner = true
	}

	isLocked := ticket.Status == models.StatusClosed || (ticket.AssignedToID != nil && *ticket.AssignedToID != user.ID)

	var assignmentHistory []models.TicketAssignmentHistory
	config.DB.Preload("Staff").Where("ticket_id = ?", ticketID).Order("assigned_at DESC").Find(&assignmentHistory)

	var rating models.TicketRating
	hasRating := false
	if ticket.Status == models.StatusClosed {
		var result models.TicketRating
		dbResult := config.DB.Preload("RatedBy").Where("ticket_id = ?", ticketID).Limit(1).Find(&result)
		if dbResult.Error == nil && dbResult.RowsAffected > 0 {
			rating = result
			hasRating = true
		}
	}

	var waitingCount int64
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusWaiting).Count(&waitingCount)

	errorMsg := r.URL.Query().Get("error")
	successMsg := r.URL.Query().Get("success")

	data := map[string]interface{}{
		"title":              "Kelola Tiket " + ticket.GetTicketNumber(),
		"page_title":          "Detail Tiket",
		"page_subtitle":       ticket.GetTicketNumber(),
		"nav_active":          "dept_dashboard",
		"template_name":       "tickets/department_ticket_detail",
		"user":                user,
		"ticket":              &ticket,
		"replies":             ticket.Replies,
		"waiting_count":       waitingCount,
		"is_owner":            isOwner,
		"is_locked":           isLocked,
		"assignment_history":  assignmentHistory,
		"has_rating":          hasRating,
		"rating":              rating,
		"success":             successMsg,
		"error":               errorMsg,
	}

	RenderTemplate(w, "tickets/department_ticket_detail", data)
}

// DepartmentReply menyimpan balasan staff ke tiket dan mengirim notif + email ke user.
func (h *DepartmentHandler) DepartmentReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path)
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = r.ParseForm()
	}
	message := strings.TrimSpace(r.FormValue("message"))
	newStatus := r.FormValue("status")

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").First(&ticket, ticketID).Error; err != nil {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tiket+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	if ticket.Status == models.StatusClosed {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Tiket+ini+sudah+ditutup+dan+tidak+bisa+dibalas", http.StatusSeeOther)
		return
	}

	if ticket.AssignedToID == nil || *ticket.AssignedToID != user.ID {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Tiket+ini+sedang+dikerjakan+oleh+staff+lain+dan+tidak+bisa+dibalas", http.StatusSeeOther)
		return
	}

	// Validate & process attachments
	attachments, err := utils.ProcessMultipartAttachments(r, "attachments", utils.DefaultUploadDir, ticket.GetTicketNumber())
	if err != nil {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	if message == "" && len(attachments) == 0 {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error="+url.QueryEscape("Pesan atau lampiran berkas harus diisi"), http.StatusSeeOther)
		return
	}
	if message == "" && len(attachments) > 0 {
		hasPDF := false
		for _, a := range attachments {
			if a.IsPDF() {
				hasPDF = true
				break
			}
		}
		if hasPDF {
			message = "[Lampiran Berkas]"
		} else {
			message = "[Lampiran Gambar]"
		}
	}

	reply := models.TicketReply{TicketID: ticket.ID, UserID: user.ID, Message: message}
	if err := config.DB.Create(&reply).Error; err != nil {
		utils.CleanupAttachments(attachments)
		log.Printf("[Staff][DepartmentReply] Gagal menyimpan balasan: %v", err)
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error="+url.QueryEscape("Gagal menyimpan balasan: "+err.Error()), http.StatusSeeOther)
		return
	}

	for i := range attachments {
		attachments[i].TicketID = ticket.ID
		attachments[i].ReplyID = &reply.ID
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			log.Printf("[Staff][DepartmentReply] Gagal menyimpan lampiran: %v", err)
			utils.CleanupAttachments(attachments[i : i+1])
		}
	}
	
	config.DB.Preload("User").Preload("Attachments").First(&reply, reply.ID)

	// Broadcast via WebSocket to connected clients (e.g. user viewing ticket)
	if h.wsHub != nil {
		var wsAtts []services.WSAttachmentPayload
		for _, a := range reply.Attachments {
			wsAtts = append(wsAtts, services.WSAttachmentPayload{
				ID:            a.ID,
				FileName:      a.FileName,
				FilePath:      a.FilePath,
				IsImage:       a.IsImage(),
				IsPDF:         a.IsPDF(),
				FormattedSize: a.GetFormattedSize(),
			})
		}
		h.wsHub.BroadcastReply(ticket.ID, &services.WSReplyPayload{
			ID:              reply.ID,
			TicketID:        ticket.ID,
			UserID:          user.ID,
			Username:        user.Username,
			UserDisplayName: user.GetFullName(),
			IsStaff:         true,
			Message:         reply.Message,
			CreatedAt:       reply.CreatedAt.Format("15:04"),
			CreatedAtISO:    reply.CreatedAt.Format(time.RFC3339),
			Attachments:     wsAtts,
		})
	}

	oldStatus := ticket.Status
	if newStatus != "" {
		ticket.Status = models.TicketStatus(newStatus)
	}

	// Feature 13: Reply First Response Safety Net
	now := time.Now()
	if ticket.FirstResponseAt == nil {
		ticket.FirstResponseAt = &now
		ticket.FirstResponseMet = CalculateFirstResponseMet(now, ticket.FirstResponseDeadline)
	}

	ticket.UpdatedAt = now
	config.DB.Save(&ticket)
	
	config.DB.Preload("CreatedBy").Preload("AssignedTo").First(&ticket, ticket.ID)

	go func() {
		models.CreateNotification(
			config.DB,
			ticket.CreatedByID,
			models.NotificationTypeReply,
			"Balasan dari tim support",
			user.GetFullName()+" membalas tiket "+ticket.GetTicketNumber()+": "+utils.TruncateString(message, 80),
			&ticket.ID,
		)
		
		if ticket.Status == models.StatusClosed && oldStatus != models.StatusClosed {
			models.CreateNotification(
				config.DB,
				ticket.CreatedByID,
				models.NotificationTypeStatusChange,
				"Tiket "+ticket.GetTicketNumber()+" selesai",
				"Tiket Anda telah ditandai selesai oleh tim support.",
				&ticket.ID,
			)
		}
	}()

	var emailAtts []utils.EmailAttachment
	for _, a := range reply.Attachments {
		emailAtts = append(emailAtts, utils.EmailAttachment{
			FileName: a.FileName,
			FilePath: a.FilePath,
			MimeType: a.MimeType,
		})
	}

	go func() {
		target := ticket.ReplyToEmail
		if target == "" {
			target = ticket.CreatedBy.GetEmail()
		}
		if target != "" {
			_ = h.emailService.SendTicketReplyWithAttachments(target, ticket.CreatedBy.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), message, user.GetFullName(), emailAtts)
		}
	}()

	http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID)), http.StatusSeeOther)
}

// escapeJSONForScript mencegah </script> di dalam JSON memutus tag script di HTML.
func escapeJSONForScript(b []byte) []byte {
	return []byte(strings.ReplaceAll(string(b), "</script>", "<\\/script>"))
}

// parseTicketIDFromPath mengurai ID tiket dari URL secara fleksibel.
func parseTicketIDFromPath(urlPath string, unused ...string) int {
	clean := strings.Trim(urlPath, "/")
	parts := strings.Split(clean, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if id, err := strconv.Atoi(parts[i]); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// ClaimTicket mengassign tiket ke staff yang login dan mencatat history.
func (h *DepartmentHandler) ClaimTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path, "/department/tiket/claim/")
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").First(&ticket, ticketID).Error; err == nil {
		var staffUser models.User
		config.DB.Select("id", "department_id", "is_super_admin").First(&staffUser, user.ID)
		if !user.IsSuperAdmin {
			if staffUser.DepartmentID == nil || ticket.DepartmentID == nil || *staffUser.DepartmentID != *ticket.DepartmentID {
				http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tidak+dapat+mengklaim+tiket+di+luar+departemen+Anda", http.StatusSeeOther)
				return
			}
		}

		wasUnassigned := ticket.AssignedToID == nil
		ticket.AssignedToID = &user.ID
		ticket.Status = models.StatusInProgress

		// Feature 12: Claim First Response Tracking
		now := time.Now()
		if ticket.FirstResponseAt == nil {
			ticket.FirstResponseAt = &now
			ticket.FirstResponseMet = CalculateFirstResponseMet(now, ticket.FirstResponseDeadline)
			isMet := false
			if ticket.FirstResponseMet != nil {
				isMet = *ticket.FirstResponseMet
			}
			logging.SLAResponses.Info("First response stamped on claim",
				"ticket_id", ticket.ID,
				"staff_id", user.ID,
				"first_response_at", now,
				"sla_met", isMet,
			)
		}

		logging.TicketLifecycle.Info("Ticket claimed by staff",
			"ticket_id", ticket.ID,
			"staff_id", user.ID,
			"username", user.Username,
			"was_unassigned", wasUnassigned,
		)

		// Optional resolution estimation parameter on claim
		_ = r.ParseForm()
		preset := strings.TrimSpace(r.FormValue("estimate_preset"))
		if preset == "" {
			preset = strings.TrimSpace(r.FormValue("preset"))
		}
		customDate := strings.TrimSpace(r.FormValue("estimated_resolution_at"))
		if customDate == "" {
			customDate = strings.TrimSpace(r.FormValue("custom_date"))
		}
		if preset != "" || customDate != "" {
			if estTime, err := ParseEstimatedResolution(preset, customDate, now); err == nil && estTime != nil {
				ticket.EstimatedResolutionAt = estTime
			}
		}

		ticket.UpdatedAt = now
		config.DB.Save(&ticket)

		history := models.TicketAssignmentHistory{
			TicketID:   ticket.ID,
			StaffID:    user.ID,
			AssignedAt: time.Now(),
		}
		config.DB.Create(&history)
		
		if wasUnassigned {
			go func() {
				models.CreateNotification(
					config.DB,
					ticket.CreatedByID,
					models.NotificationTypeTicket,
					"Tiket "+ticket.GetTicketNumber()+" sedang ditangani",
					"Tiket Anda sedang ditangani oleh "+user.GetFullName()+".",
					&ticket.ID,
				)
			}()
		}
	}

	http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?success=Tiket+berhasil+diambil.+Anda+dapat+membalas+sekarang.", http.StatusSeeOther)
}

// ReleaseTicket mengembalikan tiket ke pool (unassign) dan menandai history released.
func (h *DepartmentHandler) ReleaseTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path, "/department/tiket/release/")
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Where("id = ? AND assigned_to_id = ?", ticketID, user.ID).First(&ticket).Error; err != nil {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tiket+tidak+ditemukan+atau+bukan+milik+Anda", http.StatusSeeOther)
		return
	}

	systemReply := models.TicketReply{
		TicketID: ticket.ID,
		UserID:   user.ID,
		Message:  "Tiket dikembalikan ke pool (Released).",
	}
	config.DB.Create(&systemReply)

	now := time.Now()
	config.DB.Model(&models.TicketAssignmentHistory{}).
		Where("ticket_id = ? AND staff_id = ? AND released_at IS NULL", ticketID, user.ID).
		Update("released_at", &now)

	res := config.DB.Exec(
		"UPDATE tickets SET assigned_to_id = NULL, status = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		models.StatusWaiting, now, ticket.ID,
	)
	if res.Error != nil || res.RowsAffected == 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Gagal+melepas+tiket", http.StatusSeeOther)
		return
	}

	logging.TicketLifecycle.Info("Ticket released back to pool",
		"ticket_id", ticket.ID,
		"staff_id", user.ID,
		"correlation_id", logging.GetCorrelationID(r.Context()),
	)

	http.Redirect(w, r, config.Path("/departement/dashboard")+"?success=Tiket+berhasil+dikembalikan+ke+pool", http.StatusSeeOther)
}

// CloseTicket menutup tiket, tandai history selesai, kirim notif dan email rating ke user.
func (h *DepartmentHandler) CloseTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path, "/department/tiket/close/")
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").Where("id = ? AND assigned_to_id = ?", ticketID, user.ID).First(&ticket).Error; err == nil {
		systemReply := models.TicketReply{
			TicketID: ticket.ID,
			UserID:   user.ID,
			Message:  "Tiket ditandai selesai (Closed).",
		}
		config.DB.Create(&systemReply)

		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("ticket_id = ? AND is_completed = false", ticketID).
			Update("is_completed", true)

		oldStatus := ticket.Status
		ticket.Status = models.StatusClosed
		ticket.UpdatedAt = time.Now()
		config.DB.Save(&ticket)
		
		config.DB.Preload("CreatedBy").Preload("AssignedTo").First(&ticket, ticket.ID)

		logging.TicketLifecycle.Info("Ticket closed by staff",
			"ticket_id", ticket.ID,
			"ticket_number", ticket.GetTicketNumber(),
			"staff_id", user.ID,
			"old_status", string(oldStatus),
			"correlation_id", logging.GetCorrelationID(r.Context()),
		)

		go func() {
			if oldStatus != models.StatusClosed {
				models.CreateNotification(
					config.DB,
					ticket.CreatedByID,
					models.NotificationTypeStatusChange,
					"Tiket "+ticket.GetTicketNumber()+" selesai",
					"Tiket Anda telah ditandai selesai oleh tim support.",
					&ticket.ID,
				)
			}
		}()

		go func() {
			jwtService := utils.NewJWTService(h.cfg)
			ratingToken, err := jwtService.GenerateToken(ticket.CreatedByID, "rate_ticket", 30*24*time.Hour)
			if err != nil {
				fmt.Printf("Failed to generate rating token: %v\n", err)
				return
			}

			targetEmail := ticket.ReplyToEmail
			if targetEmail == "" {
				targetEmail = ticket.CreatedBy.GetEmail()
			}

			err = h.emailService.SendRatingRequest(
				targetEmail,
				ticket.CreatedBy.GetFullName(),
				ticket.Title,
				ticket.ID,
				ratingToken,
			)
			if err != nil {
				fmt.Printf("Failed to send rating request email: %v\n", err)
			} else {
				fmt.Printf("Rating request email sent to %s for ticket #%d\n", targetEmail, ticket.ID)
			}
		}()
	}
	http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
}

// LogoutAndRelease melepas semua tiket yang dikerjakan staff ke pool lalu redirect ke logout.
func (h *DepartmentHandler) LogoutAndRelease(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	result := config.DB.Model(&models.Ticket{}).
		Where("assigned_to_id = ? AND status = ?", user.ID, models.StatusInProgress).
		Select("assigned_to_id", "status", "updated_at").
		Updates(map[string]interface{}{
			"assigned_to_id": nil,
			"status":         models.StatusWaiting,
			"updated_at":     time.Now(),
		})
	if result.RowsAffected > 0 {
		now := time.Now()
		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("staff_id = ? AND released_at IS NULL", user.ID).
			Update("released_at", &now)
	}
	http.Redirect(w, r, config.Path("/logout"), http.StatusSeeOther)
}

// SetTicketEstimate handles quick resolution estimation updates from staff.
func (h *DepartmentHandler) SetTicketEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	isAJAX := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path)
	if ticketID <= 0 {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "ID tiket tidak valid"})
			return
		}
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").First(&ticket, ticketID).Error; err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Tiket tidak ditemukan"})
			return
		}
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tiket+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	var staffUser models.User
	config.DB.Select("id", "department_id", "is_super_admin").First(&staffUser, user.ID)
	if !user.IsSuperAdmin {
		if staffUser.DepartmentID == nil || ticket.DepartmentID == nil || *staffUser.DepartmentID != *ticket.DepartmentID {
			if isAJAX {
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Akses ditolak. Tiket berada di luar departemen Anda"})
				return
			}
			http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Akses+ditolak.+Tiket+berada+di+luar+departemen+Anda", http.StatusSeeOther)
			return
		}
	}

	if ticket.Status == models.StatusClosed {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Tiket sudah ditutup"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Tiket+sudah+ditutup", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = r.ParseForm()
	}

	preset := strings.TrimSpace(r.FormValue("preset"))
	if preset == "" {
		preset = strings.TrimSpace(r.FormValue("estimate_preset"))
	}
	customDate := strings.TrimSpace(r.FormValue("custom_date"))
	if customDate == "" {
		customDate = strings.TrimSpace(r.FormValue("estimated_resolution_at"))
	}

	estTime, err := ParseEstimatedResolution(preset, customDate, time.Now())
	if err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	ticket.EstimatedResolutionAt = estTime
	ticket.UpdatedAt = time.Now()
	if err := config.DB.Save(&ticket).Error; err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal menyimpan estimasi"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Gagal+menyimpan+estimasi", http.StatusSeeOther)
		return
	}

	if isAJAX {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":                 true,
			"message":                 "Estimasi penyelesaian berhasil disimpan",
			"estimated_resolution_at": estTime.Format(time.RFC3339),
		})
		return
	}

	http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?success=Estimasi+penyelesaian+berhasil+disimpan", http.StatusSeeOther)
}

// SetTicketPriority handles staff ticket priority adjustment with mandatory rationale, audit history, timeline reply, and notification.
func (h *DepartmentHandler) SetTicketPriority(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	isAJAX := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path)
	if ticketID <= 0 {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "ID tiket tidak valid"})
			return
		}
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").First(&ticket, ticketID).Error; err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Tiket tidak ditemukan"})
			return
		}
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Tiket+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	var staffUser models.User
	config.DB.Select("id", "department_id", "is_super_admin").First(&staffUser, user.ID)
	if !user.IsSuperAdmin {
		if staffUser.DepartmentID == nil || ticket.DepartmentID == nil || *staffUser.DepartmentID != *ticket.DepartmentID {
			if isAJAX {
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Akses ditolak. Tiket berada di luar departemen Anda"})
				return
			}
			http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Akses+ditolak.+Tiket+berada+di+luar+departemen+Anda", http.StatusSeeOther)
			return
		}
	}

	if ticket.Status == models.StatusClosed {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Tiket sudah ditutup dan tidak dapat diubah prioritasnya"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Tiket+sudah+ditutup+dan+tidak+dapat+diubah+prioritasnya", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = r.ParseForm()
	}

	newPriorityStr := strings.ToUpper(strings.TrimSpace(r.FormValue("priority")))
	newPriority := models.TicketPriority(newPriorityStr)
	reason := strings.TrimSpace(r.FormValue("reason"))

	if err := ValidatePriorityAdjustment(ticket.Priority, newPriority, reason); err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	oldPriority := ticket.Priority

	tx := config.DB.Begin()
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
		}
	}()

	// 1. Audit History
	history := models.TicketPriorityHistory{
		TicketID:    ticket.ID,
		OldPriority: oldPriority,
		NewPriority: newPriority,
		ChangedByID: user.ID,
		Reason:      reason,
		CreatedAt:   time.Now(),
	}
	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		if isAJAX {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal mencatat riwayat prioritas"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Gagal+mencatat+riwayat+prioritas", http.StatusSeeOther)
		return
	}

	// 2. Timeline System Reply
	systemReplyMsg := fmt.Sprintf("[Sistem] Prioritas tiket diubah dari %s ke %s oleh %s. Alasan: %s", oldPriority, newPriority, user.GetFullName(), reason)
	systemReply := models.TicketReply{
		TicketID:  ticket.ID,
		UserID:    user.ID,
		Message:   systemReplyMsg,
		CreatedAt: time.Now(),
	}
	if err := tx.Create(&systemReply).Error; err != nil {
		tx.Rollback()
		if isAJAX {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal membuat balasan sistem"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Gagal+membuat+balasan+sistem", http.StatusSeeOther)
		return
	}

	// 3. Update Priority & Recalculate Deadlines
	ticket.Priority = newPriority
	ticket.UpdatedAt = time.Now()

	policy, respDur, resDur := models.ResolveSLAPolicy(tx, ticket.CompanyID, ticket.DepartmentID, newPriority)
	if policy != nil && policy.ID > 0 {
		ticket.SLAPolicyID = &policy.ID
	}

	newRespDeadline, newResDeadline := RecalculateSLADeadlines(ticket.CreatedAt, ticket.FirstResponseAt, respDur, resDur)
	if newRespDeadline != nil {
		ticket.FirstResponseDeadline = newRespDeadline
	}
	ticket.ResolutionDeadline = newResDeadline

	if err := tx.Save(&ticket).Error; err != nil {
		tx.Rollback()
		if isAJAX {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal memperbarui tiket"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Gagal+memperbarui+tiket", http.StatusSeeOther)
		return
	}

	if err := tx.Commit().Error; err != nil {
		if isAJAX {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal menyimpan perubahan"})
			return
		}
		http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?error=Gagal+menyimpan+perubahan", http.StatusSeeOther)
		return
	}

	// 4. Async Notification to User
	go func() {
		notifMsg := fmt.Sprintf("Prioritas tiket %s diubah menjadi %s oleh tim support. Alasan: %s", ticket.GetTicketNumber(), newPriority, reason)
		models.CreateNotification(
			config.DB,
			ticket.CreatedByID,
			models.NotificationTypeSystem,
			"Perubahan Prioritas Tiket",
			notifMsg,
			&ticket.ID,
		)
	}()

	if isAJAX {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"message":      "Prioritas tiket berhasil diubah",
			"old_priority": oldPriority,
			"new_priority": newPriority,
		})
		return
	}

	http.Redirect(w, r, config.Path(fmt.Sprintf("/departement/tiket/%d", ticketID))+"?success=Prioritas+tiket+berhasil+diubah", http.StatusSeeOther)
}
