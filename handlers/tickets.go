package handlers

import (
	"encoding/json"
	"fmt"
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
)

type TicketHandler struct {
	cfg           *config.Config
	emailService  *utils.EmailService
	ticketService *services.TicketService
	wsHub         *services.WSHub
}

func NewTicketHandler(cfg *config.Config, emailService *utils.EmailService, ticketService *services.TicketService, wsHub ...*services.WSHub) *TicketHandler {
	var hub *services.WSHub
	if len(wsHub) > 0 {
		hub = wsHub[0]
	}
	return &TicketHandler{cfg: cfg, emailService: emailService, ticketService: ticketService, wsHub: hub}
}

func (h *TicketHandler) SetWSHub(hub *services.WSHub) {
	h.wsHub = hub
}

func (h *TicketHandler) isStaffOnline(tkt *models.Ticket) bool {
	if tkt == nil {
		return false
	}
	if tkt.AssignedToID != nil && *tkt.AssignedToID > 0 {
		if h.wsHub != nil && h.wsHub.IsUserOnline(*tkt.AssignedToID) {
			return true
		}
		var staff models.User
		if err := config.DB.Select("id", "last_active_at").First(&staff, *tkt.AssignedToID).Error; err == nil {
			return staff.IsOnline()
		}
		return false
	}
	if tkt.DepartmentID != nil && *tkt.DepartmentID > 0 {
		var count int64
		config.DB.Model(&models.User{}).
			Where("department_id = ? AND is_staff = ? AND last_active_at >= ?", *tkt.DepartmentID, true, time.Now().Add(-2*time.Minute)).
			Count(&count)
		return count > 0
	}
	return false
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
	var emailAtts []utils.EmailAttachment
	for _, a := range attachments {
		emailAtts = append(emailAtts, utils.EmailAttachment{
			FileName: a.FileName,
			FilePath: a.FilePath,
			MimeType: a.MimeType,
		})
	}
	go func() {
		logging.NotificationEmail.Info("Preparing to send ticket confirmation email",
			"ticket_id", ticket.ID,
			"ticket_number", ticket.GetTicketNumber(),
			"recipient", replyToEmail,
		)
		err := h.emailService.SendTicketConfirmationWithAttachments(replyToEmail, user.GetFullName(), ticket.Title, ticket.ID, departmentName, ticket.GetPriorityDisplay(), ticket.GetStatusDisplay(), ticket.Description, emailAtts)
		if err != nil {
			logging.NotificationEmail.Error("Failed to send ticket confirmation email",
				"ticket_id", ticket.ID,
				"ticket_number", ticket.GetTicketNumber(),
				"recipient", replyToEmail,
				"error", err.Error(),
			)
		} else {
			logging.NotificationEmail.Info("Ticket confirmation email sent successfully",
				"ticket_id", ticket.ID,
				"ticket_number", ticket.GetTicketNumber(),
				"recipient", replyToEmail,
			)
		}
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

	// Mark staff replies as read for this ticket
	readRes := config.DB.Model(&models.TicketReply{}).
		Where("ticket_id = ? AND user_id != ? AND is_read = ?", ticketID, user.ID, false).
		Updates(map[string]interface{}{
			"is_read":      true,
			"is_delivered": true,
			"read_at":      time.Now(),
		})
	if readRes.RowsAffected > 0 && h.wsHub != nil {
		h.wsHub.BroadcastMessagesRead(uint(ticketID), user.ID)
	}

	// Mark notifications for this ticket as read
	_ = config.DB.Model(&models.Notification{}).
		Where("user_id = ? AND ticket_id = ? AND is_read = ?", user.ID, ticketID, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		}).Error

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

	isDelivered := false
	isRead := false
	if h.wsHub != nil && h.wsHub.HasOtherParticipantInRoom(uint(ticketID), user.ID) {
		isDelivered = true
		isRead = true
	} else if h.isStaffOnline(&tkt) {
		isDelivered = true
	}

	reply, ticket, err := h.ticketService.AddReplyWithAttachments(uint(ticketID), user.ID, message, attachments, isDelivered, isRead)
	if err != nil {
		utils.CleanupAttachments(attachments)
		http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	log.Printf("Reply added to ticket #%d by user %s", ticketID, user.Username)

	// Broadcast via WebSocket to connected clients
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
		h.wsHub.BroadcastReply(uint(ticketID), &services.WSReplyPayload{
			ID:              reply.ID,
			TicketID:        uint(ticketID),
			UserID:          user.ID,
			Username:        user.Username,
			UserDisplayName: user.GetFullName(),
			IsStaff:         user.IsStaff,
			Message:         reply.Message,
			IsRead:          reply.IsRead,
			IsDelivered:     reply.IsDelivered,
			ReadStatus:      reply.GetReadStatusClass(),
			CreatedAt:       reply.CreatedAt.Format("15:04"),
			CreatedAtISO:    reply.CreatedAt.Format(time.RFC3339),
			Attachments:     wsAtts,
		})
	}

	var emailAtts []utils.EmailAttachment
	for _, a := range reply.Attachments {
		emailAtts = append(emailAtts, utils.EmailAttachment{
			FileName: a.FileName,
			FilePath: a.FilePath,
			MimeType: a.MimeType,
		})
	}

	if reply.UserID != ticket.CreatedByID {
		targetEmail := ticket.ReplyToEmail
		if targetEmail == "" {
			targetEmail = ticket.CreatedBy.GetEmail()
		}
		if targetEmail != "" {
			go func() {
				if err := h.emailService.SendTicketReplyWithAttachments(targetEmail, ticket.CreatedBy.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), reply.Message, user.GetFullName(), emailAtts); err != nil {
					logging.NotificationEmail.Error("Failed to send ticket reply email to creator",
						"ticket_id", ticket.ID,
						"recipient", targetEmail,
						"error", err.Error(),
					)
				}
			}()
		}
	} else if ticket.AssignedToID != nil {
		var assignedStaff models.User
		if config.DB.Select("id", "email", "username", "first_name", "last_name").First(&assignedStaff, *ticket.AssignedToID).Error == nil {
			staffEmail := assignedStaff.GetEmail()
			if staffEmail != "" {
				go func() {
					if err := h.emailService.SendTicketReplyWithAttachments(staffEmail, assignedStaff.GetFullName(), ticket.Title, ticket.ID, ticket.GetStatusDisplay(), reply.Message, user.GetFullName(), emailAtts); err != nil {
						logging.NotificationEmail.Error("Failed to send ticket reply email to assigned staff",
							"ticket_id", ticket.ID,
							"recipient", staffEmail,
							"error", err.Error(),
						)
					}
				}()
			}
		}
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
	rating, err := strconv.Atoi(r.FormValue("rating"))
	if err != nil || rating < 1 || rating > 5 {
		http.Error(w, "Invalid rating. Please select 1-5 stars", http.StatusBadRequest)
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))

	user := GetUserFromContext(r)
	var submitErr error
	if u, ok := user.(*models.User); ok && u != nil {
		submitErr = h.ticketService.SubmitRatingForUser(ticketID, u.ID, rating, comment)
	} else {
		token := r.FormValue("token")
		if token == "" {
			http.Error(w, "Rating token required", http.StatusBadRequest)
			return
		}
		submitErr = h.ticketService.SubmitRating(ticketID, token, rating, comment)
	}

	if submitErr != nil {
		if submitErr.Error() == "already rated" {
			http.Redirect(w, r, config.Path(fmt.Sprintf("/tiket/%d", ticketID))+"?error=Rating+sudah+diberikan+dan+tidak+bisa+diubah", http.StatusSeeOther)
			return
		}
		http.Error(w, "Failed to save rating: "+submitErr.Error(), http.StatusInternalServerError)
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

// GetTicketMessagesAPI returns new messages as JSON for auto-sync / polling fallback.
func (h *TicketHandler) GetTicketMessagesAPI(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/ticket/")
	path = strings.TrimSuffix(path, "/messages")
	ticketID, err := strconv.Atoi(path)
	if err != nil || ticketID <= 0 {
		ticketID, err = strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || ticketID <= 0 {
			http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
			return
		}
	}

	afterID, _ := strconv.Atoi(r.URL.Query().Get("after"))

	var ticket models.Ticket
	if err := config.DB.Select("id", "created_by_id", "department_id").First(&ticket, ticketID).Error; err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	// Authorization: SuperAdmin, Staff, or ticket creator
	if !user.IsSuperAdmin && !user.IsStaff && ticket.CreatedByID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Mark unread messages from counterpart as read when polling
	readRes := config.DB.Model(&models.TicketReply{}).
		Where("ticket_id = ? AND user_id != ? AND is_read = ?", ticketID, user.ID, false).
		Updates(map[string]interface{}{
			"is_read":      true,
			"is_delivered": true,
			"read_at":      time.Now(),
		})
	if readRes.RowsAffected > 0 && h.wsHub != nil {
		h.wsHub.BroadcastMessagesRead(uint(ticketID), user.ID)
	}

	query := config.DB.Preload("User").Preload("Attachments").Where("ticket_id = ?", ticketID)
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}

	var replies []models.TicketReply
	if err := query.Order("id ASC").Find(&replies).Error; err != nil {
		http.Error(w, "Failed to load messages", http.StatusInternalServerError)
		return
	}

	type attResp struct {
		ID            uint   `json:"id"`
		FileName      string `json:"file_name"`
		FilePath      string `json:"file_path"`
		IsImage       bool   `json:"is_image"`
		IsPDF         bool   `json:"is_pdf"`
		FormattedSize string `json:"formatted_size"`
	}

	type replyResp struct {
		ID              uint      `json:"id"`
		TicketID        uint      `json:"ticket_id"`
		UserID          uint      `json:"user_id"`
		Username        string    `json:"username"`
		UserDisplayName string    `json:"user_display_name"`
		IsStaff         bool      `json:"is_staff"`
		Message         string    `json:"message"`
		IsRead          bool      `json:"is_read"`
		IsDelivered     bool      `json:"is_delivered"`
		ReadStatus      string    `json:"read_status"`
		CreatedAt       string    `json:"created_at"`
		CreatedAtISO    string    `json:"created_at_iso"`
		Attachments     []attResp `json:"attachments"`
	}

	var respList []replyResp
	for _, rp := range replies {
		var atts []attResp
		for _, at := range rp.Attachments {
			atts = append(atts, attResp{
				ID:            at.ID,
				FileName:      at.FileName,
				FilePath:      at.FilePath,
				IsImage:       at.IsImage(),
				IsPDF:         at.IsPDF(),
				FormattedSize: at.GetFormattedSize(),
			})
		}
		respList = append(respList, replyResp{
			ID:              rp.ID,
			TicketID:        rp.TicketID,
			UserID:          rp.UserID,
			Username:        rp.User.Username,
			UserDisplayName: rp.User.GetFullName(),
			IsStaff:         rp.User.IsStaff,
			Message:         rp.Message,
			IsRead:          rp.IsRead,
			IsDelivered:     rp.IsDelivered,
			ReadStatus:      rp.GetReadStatusClass(),
			CreatedAt:       rp.CreatedAt.Format("15:04"),
			CreatedAtISO:    rp.CreatedAt.Format(time.RFC3339),
			Attachments:     atts,
		})
	}

	// Status updates for sender's outgoing messages in this ticket
	type statusItem struct {
		ID         uint   `json:"id"`
		ReadStatus string `json:"read_status"`
		Check      string `json:"check"`
		Title      string `json:"title"`
	}

	var myReplies []models.TicketReply
	config.DB.Select("id", "is_read", "is_delivered").
		Where("ticket_id = ? AND user_id = ?", ticketID, user.ID).
		Order("id DESC").
		Limit(50).
		Find(&myReplies)

	var statuses []statusItem
	for _, mr := range myReplies {
		statuses = append(statuses, statusItem{
			ID:         mr.ID,
			ReadStatus: mr.GetReadStatusClass(),
			Check:      mr.GetReadStatusCheck(),
			Title:      mr.GetReadStatusTitle(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket_id": ticketID,
		"replies":   respList,
		"statuses":  statuses,
	})
}