package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

type TicketHandler struct {
	cfg           *config.Config
	emailService  *utils.EmailService
	ticketService *services.TicketService
}

func NewTicketHandler(cfg *config.Config, emailService *utils.EmailService, ticketService *services.TicketService) *TicketHandler {
	return &TicketHandler{cfg: cfg, emailService: emailService, ticketService: ticketService}
}

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
	if h.ticketService.DepartmentCount() == 0 {
		RenderTemplate(w, "tickets/setup_error.html", map[string]interface{}{"title": "Error Konfigurasi"})
		return
	}
	departments, _ := h.ticketService.GetDepartmentsForCreate()
	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kirim Tiket Baru - Portal Ticketing",
		"page_title":    "Kirim Tiket",
		"page_subtitle": "Sampaikan kendala atau pertanyaan Anda kepada tim support kami",
		"nav_active":    "create",
		"template_name": "tickets/create_ticket",
		"departments":   departments,
		"user":          user,
	})
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

	ticket, err := h.ticketService.CreateTicket(user.ID, title, description, replyToEmail, priority, departmentID)
	if err != nil {
		log.Printf("Failed to create ticket: %v", err)
		http.Error(w, "Failed to create ticket", http.StatusInternalServerError)
		return
	}
	departmentName := "Tidak Ditentukan"
	if ticket.Department != nil {
		departmentName = ticket.Department.Name
	}
	go func() {
		log.Printf(" Mengirim email konfirmasi ke: %s", replyToEmail)
		_ = h.emailService.SendTicketConfirmation(replyToEmail, user.GetFullName(), ticket.Title, ticket.ID, departmentName, ticket.GetPriorityDisplay(), ticket.GetStatusDisplay(), ticket.Description)
	}()
	log.Printf("Ticket #%d created by user %s", ticket.ID, user.Username)
	http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/sukses/%d", ticket.ID)), http.StatusSeeOther)
}

// ShowTicketSuccess menampilkan halaman sukses
func (h *TicketHandler) ShowTicketSuccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/tiket/sukses/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
		return
	}
	ticket, err := h.ticketService.GetTicketByIDForSuccess(ticketID)
	if err != nil {
		http.Redirect(w, r, config.Path("/dashboard"), http.StatusSeeOther)
		return
	}
	RenderTemplate(w, "ticket_success.html", map[string]interface{}{"title": "Tiket Berhasil Dibuat", "ticket": ticket})
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
	tickets, _ := h.ticketService.GetMyTickets(user.ID, searchQuery, statusFilter, priorityFilter)
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
	path := strings.TrimPrefix(r.URL.Path, "/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, config.Path("/tiket"), http.StatusSeeOther)
		return
	}
	detail, err := h.ticketService.GetTicketDetailForUser(user.ID, ticketID)
	if err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}
	data := AddBaseData(r, map[string]interface{}{
		"title":         fmt.Sprintf("Tiket #%d - %s", detail.Ticket.ID, detail.Ticket.Title),
		"page_title":    fmt.Sprintf("Detail Tiket #%d", detail.Ticket.ID),
		"page_subtitle": detail.Ticket.Title,
		"nav_active":    "tickets",
		"template_name": "tickets/ticket_detail",
		"ticket":        detail.Ticket,
		"replies":       detail.Ticket.Replies,
		"has_rating":    detail.HasRating,
		"rating":        detail.Rating,
		"rating_token":  detail.RatingToken,
		"success":       r.URL.Query().Get("success"),
		"error":         r.URL.Query().Get("error"),
	})
	RenderTemplate(w, "tickets/ticket_detail", data)
}

// AddReply menambahkan reply ke tiket dan mengirim email
func (h *TicketHandler) AddReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	path := strings.TrimPrefix(r.URL.Path, "/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, config.Path("/tiket"), http.StatusSeeOther)
		return
	}
	r.ParseForm()
	message := r.FormValue("message")
	if message == "" {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID)), http.StatusSeeOther)
		return
	}
	reply, ticket, err := h.ticketService.AddReply(uint(ticketID), user.ID, message)
	if err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}
	log.Printf("Reply added to ticket #%d by user %s", ticketID, user.Username)
	if reply.UserID != ticket.CreatedByID {
		targetEmail := ticket.ReplyToEmail
		if targetEmail == "" {
			targetEmail = ticket.CreatedBy.Email
		}
		go func() {
			_ = h.emailService.SendTicketReply(targetEmail, ticket.CreatedBy.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), reply.Message, user.GetFullName())
		}()
	}
	http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID)), http.StatusSeeOther)
}

// ShowRatingForm displays the rating form for a closed ticket
func (h *TicketHandler) ShowRatingForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/rating/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Rating token required", http.StatusBadRequest)
		return
	}
	formData, err := h.ticketService.GetRatingFormData(ticketID, token)
	if err != nil {
		http.Error(w, "Invalid or expired rating token", http.StatusBadRequest)
		return
	}
	data := map[string]interface{}{
		"title":     "Rating Pengalaman - Portal Ticketing",
		"ticket":    formData.Ticket,
		"token":     token,
		"has_rated": formData.HasRated,
		"rating":    formData.Rating,
		"success":   r.URL.Query().Get("success"),
	}
	RenderTemplate(w, "tickets/rating", data)
}

// SubmitRating saves the user's rating for a closed ticket
func (h *TicketHandler) SubmitRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	path := strings.TrimPrefix(r.URL.Path, "/rating/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	if token == "" {
		http.Error(w, "Rating token required", http.StatusBadRequest)
		return
	}
	rating, err := strconv.Atoi(r.FormValue("rating"))
	if err != nil || rating < 1 || rating > 5 {
		http.Error(w, "Invalid rating. Please select 1-5 stars", http.StatusBadRequest)
		return
	}
	comment := r.FormValue("comment")
	if err := h.ticketService.SubmitRating(ticketID, token, rating, comment); err != nil {
		if err.Error() == "already rated" {
			http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error=Rating+sudah+diberikan+dan+tidak+bisa+diubah", http.StatusSeeOther)
			return
		}
		http.Error(w, "Failed to save rating", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?success=Rating+berhasil+disimpan", http.StatusSeeOther)
}

// HandleRating routes rating requests (GET for form, POST for submission)
func (h *TicketHandler) HandleRating(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowRatingForm(w, r)
	} else if r.Method == http.MethodPost {
		h.SubmitRating(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}