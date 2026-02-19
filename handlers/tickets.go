package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type TicketHandler struct {
	cfg          *config.Config
	emailService *utils.EmailService
}

func NewTicketHandler(cfg *config.Config, emailService *utils.EmailService) *TicketHandler {
	return &TicketHandler{
		cfg:          cfg,
		emailService: emailService,
	}
}

// HandleCreateTicket handles both GET and POST for create ticket
func (h *TicketHandler) HandleCreateTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowCreateTicket(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.CreateTicket(w, r)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ShowCreateTicket menampilkan form create ticket
func (h *TicketHandler) ShowCreateTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	var departmentCount int64
	config.DB.Model(&models.Department{}).Count(&departmentCount)
	if departmentCount == 0 {
		RenderTemplate(w, "tickets/setup_error.html", map[string]interface{}{
			"title": "Error Konfigurasi",
		})
		return
	}

	var departments []models.Department
	config.DB.Find(&departments)

	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kirim Tiket Baru - Portal Ticketing",
		"page_title":    "Kirim Tiket",
		"page_subtitle": "Sampaikan kendala atau pertanyaan Anda kepada tim support kami",
		"nav_active":    "create",
		"template_name": "tickets/create_ticket",
		"departments":   departments,
		"user":          user,
	})

	// PERBAIKAN: Hapus .html
	RenderTemplate(w, "tickets/create_ticket", data)
}

// CreateTicket proses pembuatan ticket baru
func (h *TicketHandler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	r.ParseForm()
	title := r.FormValue("title")
	description := r.FormValue("description")
	replyToEmail := r.FormValue("reply_to_email")
	priority := r.FormValue("priority")
	departmentIDStr := r.FormValue("department")

	if title == "" || description == "" || replyToEmail == "" {
		http.Error(w, "Semua field wajib diisi", http.StatusBadRequest)
		return
	}

	var departmentID *uint
	if departmentIDStr != "" {
		id, err := strconv.ParseUint(departmentIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			departmentID = &uid
		}
	}

	ticket := models.Ticket{
		Title:        title,
		Description:  description,
		ReplyToEmail: replyToEmail,
		Priority:     models.TicketPriority(priority),
		Status:       models.StatusWaiting,
		CreatedByID:  user.ID,
		DepartmentID: departmentID,
	}

	if err := config.DB.Create(&ticket).Error; err != nil {
		log.Printf("Failed to create ticket: %v", err)
		http.Error(w, "Failed to create ticket", http.StatusInternalServerError)
		return
	}

	config.DB.Preload("Department").First(&ticket, ticket.ID)

	departmentName := "Tidak Ditentukan"
	if ticket.Department != nil {
		departmentName = ticket.Department.Name
	}

	// Send confirmation email (Async)
	go func() {
		log.Printf(" Mengirim email konfirmasi ke: %s", replyToEmail)
		err := h.emailService.SendTicketConfirmation(
			replyToEmail,
			user.GetFullName(),
			ticket.Title,
			ticket.ID,
			departmentName,
			ticket.GetPriorityDisplay(),
			ticket.GetStatusDisplay(),
			ticket.Description,
		)
		if err != nil {
			log.Printf("Failed to send confirmation email: %v", err)
		}
	}()

	log.Printf("Ticket #%d created by user %s", ticket.ID, user.Username)

	http.Redirect(w, r, fmt.Sprintf("/tiket/sukses/%d", ticket.ID), http.StatusSeeOther)
}

// ShowTicketSuccess menampilkan halaman sukses
func (h *TicketHandler) ShowTicketSuccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/tiket/sukses/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.First(&ticket, ticketID).Error; err != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	RenderTemplate(w, "ticket_success.html", map[string]interface{}{
		"title":  "Tiket Berhasil Dibuat",
		"ticket": &ticket,
	})
}

// ShowMyTickets menampilkan daftar tiket user
func (h *TicketHandler) ShowMyTickets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)

	searchQuery := r.URL.Query().Get("search")
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "all"
	}
	priorityFilter := r.URL.Query().Get("priority")
	if priorityFilter == "" {
		priorityFilter = "all"
	}

	query := config.DB.Preload("Department").
		Preload("Replies").
		Where("created_by_id = ?", user.ID)

	if searchQuery != "" {
		if ticketID, err := strconv.Atoi(searchQuery); err == nil {
			query = query.Where("id = ? OR title LIKE ? OR description LIKE ?",
				ticketID,
				"%"+searchQuery+"%",
				"%"+searchQuery+"%")
		} else {
			query = query.Where("title LIKE ? OR description LIKE ?",
				"%"+searchQuery+"%",
				"%"+searchQuery+"%")
		}
	}

	if statusFilter != "all" {
		var status models.TicketStatus
		switch statusFilter {
		case "open":
			status = models.StatusWaiting
		case "in_progress":
			status = models.StatusInProgress
		case "closed":
			status = models.StatusClosed
		}
		query = query.Where("status = ?", status)
	}

	if priorityFilter != "all" {
		query = query.Where("priority = ?", priorityFilter)
	}

	var tickets []*models.Ticket
	query.Order("created_at DESC").Find(&tickets)

	data := AddBaseData(r, map[string]interface{}{
		"title":           "Tiket Saya - Portal Ticketing",
		"page_title":      "Tiket Saya",
		"page_subtitle":   "Kelola semua tiket support Anda",
		"nav_active":      "tickets",
		"template_name":   "tickets/my_tickets",
		"tickets":         tickets,
		"search_query":    searchQuery,
		"status_filter":   statusFilter,
		"priority_filter": priorityFilter,
	})

	RenderTemplate(w, "tickets/my_tickets", data)
}

func (h *TicketHandler) HandleTicketDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowTicketDetail(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.AddReply(w, r)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ShowTicketDetail menampilkan detail tiket
func (h *TicketHandler) ShowTicketDetail(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Extract ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, "/tiket", http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").
		Preload("Department").
		Preload("Replies.User").
		Where("id = ? AND created_by_id = ?", ticketID, user.ID).
		First(&ticket).Error; err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":         fmt.Sprintf("Tiket #%d - %s", ticket.ID, ticket.Title),
		"page_title":    fmt.Sprintf("Detail Tiket #%d", ticket.ID),
		"page_subtitle": ticket.Title,
		"nav_active":    "tickets",
		"template_name": "tickets/ticket_detail",
		"ticket":        &ticket,
		"replies":       ticket.Replies,
	})

	// PERBAIKAN: Hapus .html
	RenderTemplate(w, "tickets/ticket_detail", data)
}

// AddReply menambahkan reply ke tiket dan mengirim email
func (h *TicketHandler) AddReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)

	// Extract ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, "/tiket", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	message := r.FormValue("message")
	if message == "" {
		http.Redirect(w, r, fmt.Sprintf("/tiket/%d", ticketID), http.StatusSeeOther)
		return
	}

	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").
		Where("id = ? AND created_by_id = ?", ticketID, user.ID).
		First(&ticket).Error; err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	reply := models.TicketReply{
		TicketID: ticket.ID,
		UserID:   user.ID,
		Message:  message,
	}

	if err := config.DB.Create(&reply).Error; err != nil {
		log.Printf("Failed to create reply: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/tiket/%d", ticketID), http.StatusSeeOther)
		return
	}

	config.DB.Model(&ticket).Update("updated_at", time.Now())

	log.Printf("Reply added to ticket #%d by user %s", ticketID, user.Username)

	if reply.UserID != ticket.CreatedByID {
		targetEmail := ticket.ReplyToEmail
		if targetEmail == "" {
			targetEmail = ticket.CreatedBy.Email
		}

		go func() {
			err := h.emailService.SendTicketReply(
				targetEmail,
				ticket.CreatedBy.GetFullName(),
				ticket.Title,
				ticket.ID,
				ticket.GetStatusDisplay(),
				reply.Message,
				user.GetFullName(),
			)
			if err != nil {
				log.Printf("Failed to send reply email notification: %v", err)
			}
		}()
	}

	http.Redirect(w, r, fmt.Sprintf("/tiket/%d", ticketID), http.StatusSeeOther)
}
