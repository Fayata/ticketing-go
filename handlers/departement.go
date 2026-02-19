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

// ShowDashboard menampilkan "Ticket Pool" untuk Departemen
func (h *DepartmentHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Hitung statistik
	var waitingCount, inProgressCount, closedCount int64
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusWaiting).Count(&waitingCount)
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusInProgress).Count(&inProgressCount)
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusClosed).Count(&closedCount)

	// Ticket Pool: Ambil semua tiket yang belum Closed
	var ticketPool []*models.Ticket
	config.DB.Preload("Department").
		Preload("CreatedBy").
		Where("status != ?", models.StatusClosed).
		Order("priority = 'HIGH' DESC"). // Prioritas High paling atas
		Order("created_at ASC").         // Tiket lama dulu (FIFO)
		Find(&ticketPool)

	data := AddBaseData(r, map[string]interface{}{
		"title":          "Dashboard Departemen - Portal Ticketing",
		"page_title":     "Department Area",
		"page_subtitle":  "Kelola antrian tiket masuk",
		"nav_active":     "dept_dashboard",
		"template_name":  "tickets/department_dashboard",
		"user":           user,
		"waiting_count":  waitingCount,
		"progress_count": inProgressCount,
		"closed_count":   closedCount,
		"ticket_pool":    ticketPool,
	})

	RenderTemplate(w, "tickets/department_dashboard", data)
}

// HandleTicketDetail menangani view & reply khusus Departemen
func (h *DepartmentHandler) HandleTicketDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowTicketDetail(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.DepartmentReply(w, r)
		return
	}
}

func (h *DepartmentHandler) ShowTicketDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/department/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, "/department/dashboard", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").
		Preload("Department").
		Preload("Replies.User").
		First(&ticket, ticketID).Error; err != nil {
		http.Redirect(w, r, "/department/dashboard", http.StatusSeeOther)
		return
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":         fmt.Sprintf("Departemen: Tiket #%d", ticket.ID),
		"page_title":    fmt.Sprintf("Manage Tiket #%d", ticket.ID),
		"page_subtitle": "Dibuat oleh: " + ticket.CreatedBy.GetFullName(),
		"nav_active":    "dept_dashboard",
		"template_name": "tickets/department_ticket_detail",
		"ticket":        &ticket,
		"replies":       ticket.Replies,
	})

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

	// Simpan Reply
	reply := models.TicketReply{
		TicketID: ticket.ID,
		UserID:   user.ID,
		Message:  message,
	}
	config.DB.Create(&reply)

	// Update Status Tiket
	// Jika departemen membalas, otomatis set ke IN_PROGRESS jika masih WAITING
	if newStatus != "" {
		ticket.Status = models.TicketStatus(newStatus)
	} else if ticket.Status == models.StatusWaiting {
		ticket.Status = models.StatusInProgress
	}

	ticket.UpdatedAt = time.Now()
	config.DB.Save(&ticket)

	// Kirim Email Notifikasi ke User
	go func() {
		targetEmail := ticket.ReplyToEmail
		if targetEmail == "" {
			targetEmail = ticket.CreatedBy.Email
		}

		h.emailService.SendTicketReply(
			targetEmail,
			ticket.CreatedBy.GetFullName(),
			ticket.Title,
			ticket.ID,
			ticket.GetStatusDisplay(),
			message,
			user.GetFullName(),
		)
	}()

	http.Redirect(w, r, fmt.Sprintf("/department/tiket/%d", ticketID), http.StatusSeeOther)
}
