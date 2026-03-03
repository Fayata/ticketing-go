package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

type DepartmentHandler struct {
	cfg                 *config.Config
	emailService        *utils.EmailService
	staffDashboardService *services.StaffDashboardService
}

func NewDepartmentHandler(cfg *config.Config, emailService *utils.EmailService, staffDashboardService *services.StaffDashboardService) *DepartmentHandler {
	return &DepartmentHandler{
		cfg:                 cfg,
		emailService:        emailService,
		staffDashboardService: staffDashboardService,
	}
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

	var dbUser models.User
	if err := config.DB.Select("id", "department_id").First(&dbUser, user.ID).Error; err != nil {
		http.Error(w, "User tidak ditemukan.", http.StatusInternalServerError)
		return
	}
	if dbUser.DepartmentID == nil || *dbUser.DepartmentID == 0 {
		http.Error(w, "Akun staff belum memiliki departemen. Hubungi admin untuk assign departemen.", http.StatusForbidden)
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
		"trend_data_json":    template.JS(trendJSON),
		"monthly_data_json":  template.JS(monthlyJSON),
		"donut_data_json":    template.JS(donutJSON),
	})

	RenderTemplate(w, "tickets/department_dashboard", data)
}

// ShowAllTickets menampilkan daftar semua tiket dengan filter status dan departemen (halaman staff).
func (h *DepartmentHandler) ShowAllTickets(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

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

// DepartmentReply menyimpan balasan staff ke tiket dan mengirim notif + email ke user.
func (h *DepartmentHandler) DepartmentReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	path := strings.TrimPrefix(r.URL.Path, "/department/tiket/")
	ticketID, _ := strconv.Atoi(path)

	r.ParseForm()
	message := r.FormValue("message")
	newStatus := r.FormValue("status")

	var ticket models.Ticket
	config.DB.Preload("CreatedBy").First(&ticket, ticketID)

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
	
	config.DB.Preload("User").First(&reply, reply.ID)

	oldStatus := ticket.Status
	if newStatus != "" {
		ticket.Status = models.TicketStatus(newStatus)
	}
	ticket.UpdatedAt = time.Now()
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

	go func() {
		target := ticket.ReplyToEmail
		if target == "" {
			target = ticket.CreatedBy.Email
		}
		h.emailService.SendTicketReply(target, ticket.CreatedBy.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), message, user.GetFullName())
	}()

	http.Redirect(w, r, config.Path(fmt.Sprintf("/department/tiket/%d", ticketID)), http.StatusSeeOther)
}

// parseTicketIDFromPath mengurai ID tiket dari URL (contoh: /department/tiket/release/123 → 123).
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
		wasUnassigned := ticket.AssignedToID == nil
		ticket.AssignedToID = &user.ID
		ticket.Status = models.StatusInProgress
		ticket.UpdatedAt = time.Now()
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

	http.Redirect(w, r, config.Path("/departement/dashboard")+"?success=Tiket+berhasil+diambil.+Silakan+cek+tab+'Tiket+Saya'.", http.StatusSeeOther)
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
		Message:  "⚠️ Tiket dikembalikan ke pool (Released).",
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
			Message:  "✅ Tiket ditandai selesai (Closed).",
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
