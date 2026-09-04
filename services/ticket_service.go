package services

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type TicketService struct {
	jwtService *utils.JWTService
}

func NewTicketService(jwtService *utils.JWTService) *TicketService {
	return &TicketService{jwtService: jwtService}
}

// escapeLike escapes SQL LIKE wildcard characters to prevent wildcard injection.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// DepartmentCount returns number of departments.
func (s *TicketService) DepartmentCount() int64 {
	var n int64
	config.DB.Model(&models.Department{}).Count(&n)
	return n
}

// GetCompaniesForCreate returns all active companies for create-ticket form.
func (s *TicketService) GetCompaniesForCreate() ([]models.Company, error) {
	var list []models.Company
	err := config.DB.Where("is_active = ?", true).Order("name ASC").Find(&list).Error
	return list, err
}

// GetDepartmentsForCreate returns all departments for create-ticket form.
func (s *TicketService) GetDepartmentsForCreate() ([]models.Department, error) {
	var list []models.Department
	err := config.DB.Preload("Company").Order("name ASC").Find(&list).Error
	return list, err
}

// CreateTicket creates a new ticket and notifies staff (async). Returns created ticket with Department preloaded.
func (s *TicketService) CreateTicket(createdByID uint, title, description, replyToEmail, priority string, departmentID *uint, companyID *uint) (*models.Ticket, error) {
	return s.CreateTicketWithAttachments(createdByID, title, description, replyToEmail, priority, departmentID, companyID, nil)
}

// CreateTicketWithAttachments creates a new ticket with optional attachments.
func (s *TicketService) CreateTicketWithAttachments(createdByID uint, title, description, replyToEmail, priority string, departmentID *uint, companyID *uint, attachments []models.TicketAttachment) (*models.Ticket, error) {
	if companyID == nil && departmentID != nil {
		var dept models.Department
		if err := config.DB.Select("id", "company_id").First(&dept, *departmentID).Error; err == nil && dept.CompanyID != nil {
			companyID = dept.CompanyID
		}
	}

	ticket := models.Ticket{
		Title:        title,
		Description:  description,
		ReplyToEmail: replyToEmail,
		Priority:     models.TicketPriority(priority),
		Status:       models.StatusWaiting,
		CreatedByID:  createdByID,
		DepartmentID: departmentID,
		CompanyID:    companyID,
	}
	if err := config.DB.Create(&ticket).Error; err != nil {
		return nil, err
	}

	for i := range attachments {
		attachments[i].TicketID = ticket.ID
		attachments[i].ReplyID = nil
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			log.Printf("[TicketService] Gagal menyimpan lampiran tiket: %v", err)
		}
	}

	config.DB.Preload("Department").Preload("Company").Preload("Attachments").First(&ticket, ticket.ID)

	if ticket.DepartmentID != nil {
		go func() {
			var ticketWithUser models.Ticket
			config.DB.Preload("CreatedBy").First(&ticketWithUser, ticket.ID)
			var staffUsers []models.User
			config.DB.Where("department_id = ? AND is_staff = ? AND is_active = ?", ticket.DepartmentID, true, true).Find(&staffUsers)
			for _, staff := range staffUsers {
				models.CreateNotification(config.DB, staff.ID, models.NotificationTypeTicket,
					"Tiket baru masuk",
					ticketWithUser.GetTicketNumber()+" dari \""+ticketWithUser.CreatedBy.Username+"\" membutuhkan penanganan segera.",
					&ticket.ID)
			}
		}()
	}
	return &ticket, nil
}

// GetTicketByIDForSuccess returns ticket by ID (for success page). Nil if not found.
func (s *TicketService) GetTicketByIDForSuccess(id int) (*models.Ticket, error) {
	var ticket models.Ticket
	if err := config.DB.First(&ticket, id).Error; err != nil {
		return nil, err
	}
	return &ticket, nil
}

// GetMyTickets returns filtered tickets for user.
func (s *TicketService) GetMyTickets(userID uint, searchQuery, statusFilter, priorityFilter string) ([]*models.Ticket, error) {
	query := config.DB.Preload("Department").Preload("Replies").Where("created_by_id = ?", userID)

	if searchQuery != "" {
		log.Printf("[Security][SQLi] Search query sanitized: original=%q escaped=%q", searchQuery, escapeLike(searchQuery))
		cleanSearch := strings.TrimPrefix(strings.ToUpper(searchQuery), "T")
		if ticketID, err := strconv.Atoi(cleanSearch); err == nil {
			query = query.Where("id = ? OR title LIKE ? OR description LIKE ?", ticketID, "%"+escapeLike(searchQuery)+"%", "%"+escapeLike(searchQuery)+"%")
		} else {
			if len(cleanSearch) > 2 {
				potentialIDStr := cleanSearch[2:]
				if potentialID, err := strconv.Atoi(potentialIDStr); err == nil {
					query = query.Where("id = ? OR title LIKE ? OR description LIKE ?", potentialID, "%"+escapeLike(searchQuery)+"%", "%"+escapeLike(searchQuery)+"%")
				} else {
					query = query.Where("title LIKE ? OR description LIKE ?", "%"+escapeLike(searchQuery)+"%", "%"+escapeLike(searchQuery)+"%")
				}
			} else {
				query = query.Where("title LIKE ? OR description LIKE ?", "%"+escapeLike(searchQuery)+"%", "%"+escapeLike(searchQuery)+"%")
			}
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
	err := query.Order("created_at DESC").Find(&tickets).Error
	return tickets, err
}

// TicketDetailForUser holds ticket detail data for user view.
type TicketDetailForUser struct {
	Ticket      *models.Ticket
	HasRating   bool
	Rating      models.TicketRating
	RatingToken string
}

// GetTicketDetailForUser returns ticket if it belongs to user. Rating info included when closed.
func (s *TicketService) GetTicketDetailForUser(userID uint, ticketID int) (*TicketDetailForUser, error) {
	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").Preload("Department").Preload("Replies.User").Preload("Replies.Attachments").Preload("Attachments").
		Where("id = ? AND created_by_id = ?", ticketID, userID).First(&ticket).Error; err != nil {
		return nil, err
	}
	out := &TicketDetailForUser{Ticket: &ticket}
	if ticket.Status == models.StatusClosed {
		var rating models.TicketRating
		if config.DB.Where("ticket_id = ?", ticketID).Limit(1).Find(&rating).Error == nil && rating.ID != 0 {
			out.HasRating = true
			out.Rating = rating
		} else if ticket.CreatedByID == userID {
			token, _ := s.jwtService.GenerateToken(userID, "rate_ticket", 30*24*time.Hour)
			out.RatingToken = token
		}
	}
	return out, nil
}

// AddReply adds a reply to user's ticket and notifies staff. Returns reply and ticket for email.
func (s *TicketService) AddReply(ticketID uint, userID uint, message string) (reply *models.TicketReply, ticket *models.Ticket, err error) {
	return s.AddReplyWithAttachments(ticketID, userID, message, nil)
}

// AddReplyWithAttachments adds a reply with attachments to user's ticket.
func (s *TicketService) AddReplyWithAttachments(ticketID uint, userID uint, message string, attachments []models.TicketAttachment) (reply *models.TicketReply, ticket *models.Ticket, err error) {
	var tkt models.Ticket
	if err := config.DB.Preload("CreatedBy").Where("id = ? AND created_by_id = ?", ticketID, userID).First(&tkt).Error; err != nil {
		return nil, nil, err
	}
	if tkt.Status == models.StatusClosed {
		return nil, nil, errors.New("tiket ini sudah ditutup dan tidak bisa dibalas")
	}
	reply = &models.TicketReply{TicketID: tkt.ID, UserID: userID, Message: message}
	if err := config.DB.Create(reply).Error; err != nil {
		return nil, nil, err
	}

	for i := range attachments {
		attachments[i].TicketID = tkt.ID
		attachments[i].ReplyID = &reply.ID
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			log.Printf("[TicketService] Gagal menyimpan lampiran balasan: %v", err)
		}
	}

	config.DB.Preload("User").Preload("Attachments").First(reply, reply.ID)
	config.DB.Model(&tkt).Update("updated_at", time.Now())

	var user models.User
	config.DB.First(&user, userID)
	if !user.IsStaff && tkt.AssignedToID != nil {
		go func() {
			models.CreateNotification(config.DB, *tkt.AssignedToID, models.NotificationTypeReply,
				"Balasan dari pengguna",
				user.GetFullName()+" membalas tiket "+tkt.GetTicketNumber()+": "+utils.TruncateString(reply.Message, 80),
				&tkt.ID)
		}()
	}
	return reply, &tkt, nil
}

// RatingFormData for rating page.
type RatingFormData struct {
	Ticket   *models.Ticket
	HasRated bool
	Rating   models.TicketRating
}

// GetRatingFormData returns data for rating form. Token must be valid for rate_ticket.
func (s *TicketService) GetRatingFormData(ticketID int, token string) (*RatingFormData, error) {
	claims, err := s.jwtService.ValidateToken(token)
	if err != nil || claims.Purpose != "rate_ticket" {
		return nil, errors.New("invalid token")
	}
	var ticket models.Ticket
	if err := config.DB.Preload("CreatedBy").Preload("Department").
		Where("id = ? AND created_by_id = ? AND status = ?", ticketID, claims.UserID, models.StatusClosed).
		First(&ticket).Error; err != nil {
		return nil, err
	}
	var existingRating models.TicketRating
	hasRated := config.DB.Where("ticket_id = ?", ticketID).First(&existingRating).Error == nil
	return &RatingFormData{Ticket: &ticket, HasRated: hasRated, Rating: existingRating}, nil
}

// SubmitRating saves rating. Returns nil on success.
func (s *TicketService) SubmitRating(ticketID int, token string, rating int, comment string) error {
	claims, err := s.jwtService.ValidateToken(token)
	if err != nil || claims.Purpose != "rate_ticket" {
		return errors.New("invalid token")
	}
	var ticket models.Ticket
	if err := config.DB.Where("id = ? AND created_by_id = ? AND status = ?", ticketID, claims.UserID, models.StatusClosed).First(&ticket).Error; err != nil {
		return err
	}
	var existing models.TicketRating
	if config.DB.Where("ticket_id = ?", ticketID).First(&existing).Error == nil {
		return errors.New("already rated")
	}
	newRating := models.TicketRating{
		TicketID:  uint(ticketID),
		Rating:    rating,
		Comment:   comment,
		RatedByID: claims.UserID,
		RatedAt:   time.Now(),
	}
	return config.DB.Create(&newRating).Error
}
