package handlers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
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

// HandleCreateTicket mengarahkan GET ke form buat tiket, POST ke proses simpan.
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

// ShowCreateTicket menampilkan form kirim tiket baru (user portal).
func (h *TicketHandler) ShowCreateTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	if h.ticketService.DepartmentCount() == 0 {
		RenderTemplate(w, "tickets/setup_error.html", map[string]interface{}{"title": "Error Konfigurasi"})
		return
	}
	companies, _ := h.ticketService.GetCompaniesForCreate()
	departments, _ := h.ticketService.GetDepartmentsForCreate()
	data := AddBaseData(r, map[string]interface{}{
		"title":         "Kirim Tiket Baru - Portal Ticketing",
		"page_title":    "Kirim Tiket",
		"page_subtitle": "Sampaikan kendala atau pertanyaan Anda kepada tim support kami",
		"nav_active":    "create",
		"template_name": "tickets/create_ticket",
		"companies":     companies,
		"departments":   departments,
		"user":          user,
		"error":         r.URL.Query().Get("error"),
	})
	RenderTemplate(w, "tickets/create_ticket", data)
}

// CreateTicket menyimpan tiket baru dan mengirim email konfirmasi ke user.
func (h *TicketHandler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = r.ParseForm()
	}
	title := r.FormValue("title")
	description := r.FormValue("description")
	replyToEmail := r.FormValue("reply_to_email")
	priority := r.FormValue("priority")
	companyIDStr := r.FormValue("company_id")
	departmentIDStr := r.FormValue("department")
	if title == "" || description == "" || replyToEmail == "" {
		http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Semua field wajib diisi"), http.StatusSeeOther)
		return
	}

	// Pre-validate file attachments before creating ticket in DB
	if err := utils.ValidateMultipartRequest(r, "attachments"); err != nil {
		http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	var companyID *uint
	if companyIDStr != "" {
		id, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			companyID = &uid
		}
	}
	var departmentID *uint
	if departmentIDStr != "" {
		id, err := strconv.ParseUint(departmentIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			departmentID = &uid
		}
	}

	// Pre-validate attachments before ticket creation to prevent auto-increment ID sequence gaps on invalid files
	if valErr := utils.ValidateMultipartRequest(r, "attachments"); valErr != nil {
		http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape(valErr.Error()), http.StatusSeeOther)
		return
	}

	ticket, err := h.ticketService.CreateTicket(user.ID, title, description, replyToEmail, priority, departmentID, companyID)
	if err != nil {
		log.Printf("Failed to create ticket: %v", err)
		http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape("Gagal membuat tiket: "+err.Error()), http.StatusSeeOther)
		return
	}

	attachments, err := utils.ProcessMultipartAttachments(r, "attachments", utils.DefaultUploadDir, ticket.GetTicketNumber())
	if err != nil {
		config.DB.Where("ticket_id = ?", ticket.ID).Delete(&models.Notification{})
		config.DB.Delete(ticket)
		http.Redirect(w, r, config.Path("/kirim-tiket")+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	for i := range attachments {
		attachments[i].TicketID = ticket.ID
		attachments[i].ReplyID = nil
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			log.Printf("[TicketHandler] Gagal menyimpan lampiran tiket: %v", err)
			utils.CleanupAttachments(attachments[i : i+1])
		}
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

// ShowTicketSuccess menampilkan halaman sukses setelah tiket berhasil dibuat.
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

// ShowMyTickets menampilkan daftar tiket milik user yang login.
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

// HandleTicketDetail mengarahkan GET ke detail tiket, POST ke tambah balasan.
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

// ShowTicketDetail menampilkan halaman detail tiket untuk user (lihat, balas).
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
		"title":         fmt.Sprintf("Tiket %s - %s", detail.Ticket.GetTicketNumber(), detail.Ticket.Title),
		"page_title":    "Detail Tiket",
		"page_subtitle": detail.Ticket.GetTicketNumber(),
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

// AddReply menyimpan balasan user ke tiket dan mengirim notif/email ke staff.
func (h *TicketHandler) AddReply(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	path := strings.TrimPrefix(r.URL.Path, "/tiket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil {
		http.Redirect(w, r, config.Path("/tiket"), http.StatusSeeOther)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = r.ParseForm()
	}
	message := strings.TrimSpace(r.FormValue("message"))

	var tkt models.Ticket
	if err := config.DB.Preload("CreatedBy").Where("id = ? AND created_by_id = ?", ticketID, user.ID).First(&tkt).Error; err != nil {
		http.Redirect(w, r, config.Path("/tiket"), http.StatusSeeOther)
		return
	}
	if tkt.Status == models.StatusClosed {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error=Tiket+ini+sudah+ditutup+dan+tidak+bisa+dibalas", http.StatusSeeOther)
		return
	}

	// Validate & process attachments
	attachments, err := utils.ProcessMultipartAttachments(r, "attachments", utils.DefaultUploadDir, tkt.GetTicketNumber())
	if err != nil {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	if message == "" && len(attachments) == 0 {
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error="+url.QueryEscape("Pesan atau lampiran berkas harus diisi"), http.StatusSeeOther)
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

	reply, ticket, err := h.ticketService.AddReplyWithAttachments(uint(ticketID), user.ID, message, attachments)
	if err != nil {
		utils.CleanupAttachments(attachments)
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
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

// ShowRatingForm menampilkan form penilaian tiket (akses via token di URL).
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

// SubmitRating menyimpan rating user untuk tiket yang sudah ditutup.
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

// HandleRating mengarahkan GET ke form rating, POST ke simpan rating (pakai token di URL).
func (h *TicketHandler) HandleRating(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ShowRatingForm(w, r)
	} else if r.Method == http.MethodPost {
		h.SubmitRating(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}