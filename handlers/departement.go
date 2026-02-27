package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type DepartmentHandler struct {
	cfg          *config.Config
	emailService *utils.EmailService
}

func NewDepartmentHandler(cfg *config.Config, emailService *utils.EmailService) *DepartmentHandler {
	return &DepartmentHandler{
		cfg:          cfg,
		emailService: emailService,
	}
}

// bantu isi data base buat halaman departemen
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

// Dashboard staff: statistik, tiket saya, pool, chart
func (h *DepartmentHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Pastikan department_id ter-load dari DB (kadang context user kurang lengkap)
	var dbUser models.User
	if err := config.DB.Select("id", "department_id").First(&dbUser, user.ID).Error; err != nil {
		http.Error(w, "User tidak ditemukan.", http.StatusInternalServerError)
		return
	}
	if dbUser.DepartmentID == nil {
		http.Error(w, "Akun staff belum memiliki departemen. Hubungi admin.", http.StatusForbidden)
		return
	}
	deptID := *dbUser.DepartmentID

	// card statistik: waiting = pool (belum di-claim), in progress, closed
	var waitingCount, inProgressCount, closedCount int64
	config.DB.Model(&models.Ticket{}).
		Where("assigned_to_id IS NULL AND status = ? AND (department_id = ? OR department_id IS NULL)", models.StatusWaiting, deptID).
		Count(&waitingCount)
	config.DB.Model(&models.Ticket{}).
		Where("status = ? AND department_id = ?", models.StatusInProgress, deptID).
		Count(&inProgressCount)
	config.DB.Model(&models.Ticket{}).
		Where("status = ? AND department_id = ?", models.StatusClosed, deptID).
		Count(&closedCount)

	// tiket yang di-assign ke staff ini
	var myActiveTickets []*models.Ticket
	config.DB.Preload("Department").Preload("CreatedBy").
		Where("assigned_to_id = ? AND status != ?", user.ID, models.StatusClosed).
		Order("updated_at DESC").
		Find(&myActiveTickets)

	// pool = tiket yang belum di-claim (assigned_to_id NULL, status WAITING) untuk departemen saya atau umum
	var ticketPool []*models.Ticket
	config.DB.Preload("Department").Preload("CreatedBy").
		Where("assigned_to_id IS NULL AND status = ? AND (department_id = ? OR department_id IS NULL)", models.StatusWaiting, deptID).
		Order("created_at ASC").
		Find(&ticketPool)

	// data buat chart
	currentYear := time.Now().Year()
	activityMap := make(map[int]map[uint]bool)
	for i := 1; i <= 12; i++ {
		activityMap[i] = make(map[uint]bool)
	}

	var replies []models.TicketReply
	config.DB.Where("user_id = ? AND EXTRACT(YEAR FROM created_at) = ?", user.ID, currentYear).Find(&replies)

	for _, reply := range replies {
		month := int(reply.CreatedAt.Month())
		activityMap[month][reply.TicketID] = true
	}

	months := []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Ags", "Sep", "Okt", "Nov", "Des"}
	var chartData []MonthlyStat
	maxCount := 0
	for i := 1; i <= 12; i++ {
		if len(activityMap[i]) > maxCount {
			maxCount = len(activityMap[i])
		}
	}

	for i, monthName := range months {
		count := len(activityMap[i+1])
		height := "0%"
		if maxCount > 0 {
			pct := (float64(count) / float64(maxCount)) * 100
			if pct > 0 && pct < 5 {
				pct = 5
			}
			height = fmt.Sprintf("%.0f%%", pct)
		}
		chartData = append(chartData, MonthlyStat{MonthLabel: monthName, Count: count, Height: height})
	}

	// Trend Analysis
	thisMonth := int(time.Now().Month())
	lastMonth := thisMonth - 1
	thisMonthCount := len(activityMap[thisMonth])
	lastMonthCount := 0
	if lastMonth > 0 {
		lastMonthCount = len(activityMap[lastMonth])
	}

	var trendLabel, trendColor string
	if lastMonthCount == 0 {
		if thisMonthCount > 0 {
			trendLabel = "+100% dari bulan lalu"
			trendColor = "#16a34a"
		} else {
			trendLabel = "0% dari bulan lalu"
			trendColor = "#6b7280"
		}
	} else {
		diff := float64(thisMonthCount - lastMonthCount)
		pct := (diff / float64(lastMonthCount)) * 100
		if pct > 0 {
			trendLabel = fmt.Sprintf("+%.0f%% dari bulan lalu", pct)
			trendColor = "#16a34a"
		} else if pct < 0 {
			trendLabel = fmt.Sprintf("%.0f%% dari bulan lalu", pct)
			trendColor = "#dc2626"
		} else {
			trendLabel = "0% dari bulan lalu"
			trendColor = "#6b7280"
		}
	}

	successMsg := r.URL.Query().Get("success")

	data := h.addDepartmentData(r, map[string]interface{}{
		"title":             "Dashboard Departemen",
		"page_title":        "Department Area",
		"nav_active":        "dept_dashboard",
		"template_name":     "tickets/department_dashboard",
		"user":              user,
		"waiting_count":     waitingCount,
		"progress_count":    inProgressCount,
		"closed_count":      closedCount,
		"ticket_pool":       ticketPool,
		"my_active_tickets": myActiveTickets,
		"chart_data":        chartData,
		"trend_label":       trendLabel,
		"trend_color":       trendColor,
		"success":           successMsg,
	})

	RenderTemplate(w, "tickets/department_dashboard", data)
}

func (h *DepartmentHandler) ShowAllTickets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Ambil Parameter Filter
	statusFilter := r.URL.Query().Get("status")
	deptFilter := r.URL.Query().Get("department")
	query := config.DB.Preload("Department").Preload("CreatedBy").Preload("AssignedTo").Model(&models.Ticket{})

	if statusFilter != "" && statusFilter != "ALL" {
		query = query.Where("status = ?", statusFilter)
	}

	if deptFilter != "" && deptFilter != "ALL" {
		query = query.Where("department_id = ?", deptFilter)
	}

	var tickets []*models.Ticket
	query.Order("created_at DESC").Find(&tickets)

	// Load ratings for closed tickets
	ticketIDs := make([]uint, 0, len(tickets))
	for _, t := range tickets {
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

	// Ambil Data Departemen untuk Dropdown Filter
	var departments []models.Department
	config.DB.Find(&departments)

	data := h.addDepartmentData(r, map[string]interface{}{
		"title":         "Semua Tiket - Department",
		"page_title":    "Semua Tiket",
		"page_subtitle": "Daftar seluruh tiket yang masuk ke sistem",
		"nav_active":    "dept_all_tickets",
		"template_name": "tickets/department_all_tickets",
		"user":          user,
		"tickets":       tickets,
		"departments":   departments,
		"filter_status": statusFilter,
		"filter_dept":   deptFilter,
		"ratings_map":   ratingsMap,
	})

	RenderTemplate(w, "tickets/department_all_tickets", data)
}

func (h *DepartmentHandler) HandleTicketDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowTicketDetail(w, r)
	} else if r.Method == http.MethodPost {
		h.DepartmentReply(w, r)
	}
}

func (h *DepartmentHandler) ShowTicketDetail(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	path := strings.TrimPrefix(r.URL.Path, "/department/tiket/")
	ticketID, _ := strconv.Atoi(path)

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").Preload("Department").Preload("Replies.User").Preload("AssignedTo").
		First(&ticket, ticketID).Error; err != nil {
		http.Redirect(w, r, config.Path("/departement/dashboard"), http.StatusSeeOther)
		return
	}
	isOwner := false
	if ticket.AssignedToID != nil && *ticket.AssignedToID == user.ID {
		isOwner = true
	}

	// Check if ticket is locked (CLOSED or assigned to another staff)
	isLocked := ticket.Status == models.StatusClosed || (ticket.AssignedToID != nil && *ticket.AssignedToID != user.ID)

	// Load assignment history
	var assignmentHistory []models.TicketAssignmentHistory
	config.DB.Preload("Staff").Where("ticket_id = ?", ticketID).Order("assigned_at DESC").Find(&assignmentHistory)

	// Load rating if ticket is closed (ignore record not found without logging error)
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

	// Get error message from query parameter if any
	errorMsg := r.URL.Query().Get("error")

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
		"error":               errorMsg,
	}

	RenderTemplate(w, "tickets/department_ticket_detail", data)
}

func (h *DepartmentHandler) DepartmentReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	path := strings.TrimPrefix(r.URL.Path, "/department/tiket/")
	ticketID, _ := strconv.Atoi(path)

	r.ParseForm()
	message := r.FormValue("message")
	newStatus := r.FormValue("status")

	var ticket models.Ticket
	config.DB.Preload("CreatedBy").First(&ticket, ticketID)

	// Lock check: Prevent reply if ticket is CLOSED or assigned to another staff
	if ticket.Status == models.StatusClosed {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/department/tiket/%d", ticketID))+"?error=Tiket+ini+sudah+ditutup+dan+tidak+bisa+dibalas", http.StatusSeeOther)
		return
	}

	if ticket.AssignedToID == nil || *ticket.AssignedToID != user.ID {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/department/tiket/%d", ticketID))+"?error=Tiket+ini+sedang+dikerjakan+oleh+staff+lain+dan+tidak+bisa+dibalas", http.StatusSeeOther)
		return
	}

	reply := models.TicketReply{TicketID: ticket.ID, UserID: user.ID, Message: message}
	config.DB.Create(&reply)
	
	// Load reply with user
	config.DB.Preload("User").First(&reply, reply.ID)

	oldStatus := ticket.Status
	if newStatus != "" {
		ticket.Status = models.TicketStatus(newStatus)
	}
	ticket.UpdatedAt = time.Now()
	config.DB.Save(&ticket)
	
	// Load ticket with relations
	config.DB.Preload("CreatedBy").Preload("AssignedTo").First(&ticket, ticket.ID)

	// Create notification for ticket owner
	go func() {
		models.CreateNotification(
			config.DB,
			ticket.CreatedByID,
			models.NotificationTypeReply,
			"Balasan dari tim support",
			user.GetFullName()+" membalas tiket "+ticket.GetTicketNumber()+": "+utils.TruncateString(message, 80),
			&ticket.ID,
		)
		
		// Create notification for status change
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

	go func() {
		target := ticket.ReplyToEmail
		if target == "" {
			target = ticket.CreatedBy.Email
		}
		h.emailService.SendTicketReply(target, ticket.CreatedBy.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), message, user.GetFullName())
	}()

	http.Redirect(w, r, config.Path(fmt.Sprintf("/department/tiket/%d", ticketID)), http.StatusSeeOther)
}

// parseTicketIDFromPath mengambil ID tiket dari path (misal "/department/tiket/release/123" -> 123)
func parseTicketIDFromPath(urlPath, prefix string) int {
	path := strings.TrimPrefix(urlPath, prefix)
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0
	}
	id, _ := strconv.Atoi(parts[0])
	return id
}

func (h *DepartmentHandler) ClaimTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path, "/department/tiket/claim/")
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").First(&ticket, ticketID).Error; err == nil {
		wasUnassigned := ticket.AssignedToID == nil
		ticket.AssignedToID = &user.ID
		ticket.Status = models.StatusInProgress
		ticket.UpdatedAt = time.Now()
		config.DB.Save(&ticket)

		// Record assignment history
		history := models.TicketAssignmentHistory{
			TicketID:   ticket.ID,
			StaffID:    user.ID,
			AssignedAt: time.Now(),
		}
		config.DB.Create(&history)
		
		// Create notification for ticket owner
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

	http.Redirect(w, r, config.Path("/departement/dashboard")+"?success=Tiket+berhasil+diambil.+Silakan+cek+tab+'Tiket+Saya'.", http.StatusSeeOther)
}

func (h *DepartmentHandler) ReleaseTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	ticketID := parseTicketIDFromPath(r.URL.Path, "/department/tiket/release/")
	if ticketID <= 0 {
		http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=ID+tiket+tidak+valid", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Where("id = ? AND assigned_to_id = ?", ticketID, user.ID).First(&ticket).Error; err == nil {
		systemReply := models.TicketReply{
			TicketID: ticket.ID,
			UserID:   user.ID,
			Message:  "⚠️ Tiket dikembalikan ke pool (Released).",
		}
		config.DB.Create(&systemReply)

		// Update assignment history - mark as released
		now := time.Now()
		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("ticket_id = ? AND staff_id = ? AND released_at IS NULL", ticketID, user.ID).
			Update("released_at", &now)

		ticket.AssignedToID = nil
		ticket.Status = models.StatusWaiting
		ticket.UpdatedAt = time.Now()
		if err := config.DB.Save(&ticket).Error; err != nil {
			http.Redirect(w, r, config.Path("/departement/dashboard")+"?error=Gagal+melepas+tiket", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, config.Path("/departement/dashboard")+"?success=Tiket+berhasil+dikembalikan+ke+pool", http.StatusSeeOther)
}

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
			Message:  "✅ Tiket ditandai selesai (Closed).",
		}
		config.DB.Create(&systemReply)

		// Mark all staff who worked on this ticket as completed
		// This includes current staff and any previous staff who released it
		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("ticket_id = ? AND is_completed = false", ticketID).
			Update("is_completed", true)

		oldStatus := ticket.Status
		ticket.Status = models.StatusClosed
		ticket.UpdatedAt = time.Now()
		config.DB.Save(&ticket)
		
		// Load ticket with relations
		config.DB.Preload("CreatedBy").Preload("AssignedTo").First(&ticket, ticket.ID)

		// Create notification for status change
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

		// Send rating request email to user
		go func() {
			// Generate rating token (valid for 30 days)
			jwtService := utils.NewJWTService(h.cfg)
			ratingToken, err := jwtService.GenerateToken(ticket.CreatedByID, "rate_ticket", 30*24*time.Hour)
			if err != nil {
				fmt.Printf("Failed to generate rating token: %v\n", err)
				return
			}

			// Get user email
			targetEmail := ticket.ReplyToEmail
			if targetEmail == "" {
				targetEmail = ticket.CreatedBy.Email
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

func (h *DepartmentHandler) LogoutAndRelease(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	// Lepas semua tiket yang sedang dikerjakan staff ini ke pool (assigned_to_id = NULL, status = WAITING)
	result := config.DB.Model(&models.Ticket{}).
		Where("assigned_to_id = ? AND status = ?", user.ID, models.StatusInProgress).
		Select("assigned_to_id", "status", "updated_at").
		Updates(map[string]interface{}{
			"assigned_to_id": nil,
			"status":         models.StatusWaiting,
			"updated_at":     time.Now(),
		})
	if result.RowsAffected > 0 {
		// Tandai assignment history yang belum released
		now := time.Now()
		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("staff_id = ? AND released_at IS NULL", user.ID).
			Update("released_at", &now)
	}
	http.Redirect(w, r, config.Path("/logout"), http.StatusSeeOther)
}
