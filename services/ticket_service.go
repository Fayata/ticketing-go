package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
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

	// Normalize priority with fallback to MEDIUM
	ticketPriority := models.TicketPriority(strings.ToUpper(strings.TrimSpace(priority)))
	if ticketPriority != models.PriorityHigh && ticketPriority != models.PriorityLow && ticketPriority != models.PriorityMedium {
		ticketPriority = models.PriorityMedium
	}

	// 24/7 Calendar continuous calculation: resolve policy and calculate deadlines
	now := time.Now()
	policy, respDuration, resDuration := models.ResolveSLAPolicy(config.DB, companyID, departmentID, ticketPriority)

	firstResponseDeadline := now.Add(respDuration)
	resolutionDeadline := now.Add(resDuration)

	ticket := models.Ticket{
		Title:                 title,
		Description:           description,
		ReplyToEmail:          replyToEmail,
		Priority:              ticketPriority,
		Status:                models.StatusWaiting,
		CreatedByID:           createdByID,
		DepartmentID:          departmentID,
		CompanyID:             companyID,
		CreatedAt:             now,
		FirstResponseDeadline: &firstResponseDeadline,
		ResolutionDeadline:    &resolutionDeadline,
		FirstResponseAt:       nil,
		FirstResponseMet:      nil,
		SLAWarningSent:        false,
		SLABreachSent:         false,
	}
	if policy != nil && policy.ID > 0 {
		ticket.SLAPolicyID = &policy.ID
	}

	if err := config.DB.Create(&ticket).Error; err != nil {
		logging.TicketLifecycle.Error("Failed to create ticket in database",
			"error", err.Error(),
			"title", title,
			"created_by_id", createdByID,
		)
		return nil, err
	}

	logging.TicketLifecycle.Info("Ticket created",
		"ticket_id", ticket.ID,
		"ticket_number", ticket.GetTicketNumber(),
		"title", ticket.Title,
		"created_by_id", createdByID,
		"priority", string(ticketPriority),
	)

	logging.SLACalculations.Info("SLA policy calculated for new ticket",
		"ticket_id", ticket.ID,
		"ticket_number", ticket.GetTicketNumber(),
		"priority", string(ticketPriority),
		"first_response_deadline", firstResponseDeadline,
		"resolution_deadline", resolutionDeadline,
		"response_target_hours", respDuration.Hours(),
	)

	for i := range attachments {
		attachments[i].TicketID = ticket.ID
		attachments[i].ReplyID = nil
		ticketNumClean := strings.TrimSpace(ticket.GetTicketNumber())
		ticketNumClean = strings.ReplaceAll(ticketNumClean, " ", "")
		base := filepath.Base(attachments[i].FilePath)
		if attachments[i].FilePath != "" && !strings.HasPrefix(base, ticketNumClean+"-") {
			oldPath := filepath.FromSlash(attachments[i].FilePath)
			timestamp := time.Now().Unix()
			newName := utils.GenerateTicketAttachmentFileName(ticketNumClean, timestamp, i+1, len(attachments), attachments[i].FileName)
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			if _, err := os.Stat(newPath); err == nil {
				ext := filepath.Ext(newName)
				baseName := strings.TrimSuffix(newName, ext)
				counter := 1
				for {
					candidateName := fmt.Sprintf("%s_%d%s", baseName, counter, ext)
					candPath := filepath.Join(filepath.Dir(oldPath), candidateName)
					if _, err := os.Stat(candPath); os.IsNotExist(err) {
						newName = candidateName
						newPath = candPath
						break
					}
					counter++
				}
			}
			if _, err := os.Stat(oldPath); err == nil {
				_ = os.Rename(oldPath, newPath)
			}
			attachments[i].FilePath = filepath.ToSlash(newPath)
		}
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			logging.TicketAttachments.Error("Failed to save ticket attachment",
				"ticket_id", ticket.ID,
				"file_name", attachments[i].FileName,
				"error", err.Error(),
			)
			utils.CleanupAttachments(attachments[i : i+1])
		} else {
			logging.TicketAttachments.Info("Ticket attachment saved",
				"ticket_id", ticket.ID,
				"attachment_id", attachments[i].ID,
				"file_name", attachments[i].FileName,
				"file_path", attachments[i].FilePath,
				"file_size", attachments[i].FileSize,
				"is_pdf", attachments[i].IsPDF(),
			)
		}
	}

	config.DB.Preload("Department").Preload("Company").Preload("Attachments").Preload("SLAPolicy").First(&ticket, ticket.ID)

	if ticket.DepartmentID != nil {
		go func() {
			var ticketWithUser models.Ticket
			if err := config.DB.Preload("CreatedBy").First(&ticketWithUser, ticket.ID).Error; err != nil {
				return
			}
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
	if err == nil {
		for _, t := range tickets {
			unread := 0
			for _, r := range t.Replies {
				if r.UserID != userID && !r.IsRead {
					unread++
				}
			}
			t.UnreadCount = unread
		}
	}
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
func (s *TicketService) AddReplyWithAttachments(ticketID uint, userID uint, message string, attachments []models.TicketAttachment, deliveryStatus ...bool) (reply *models.TicketReply, ticket *models.Ticket, err error) {
	var tkt models.Ticket
	if err := config.DB.Preload("CreatedBy").Where("id = ? AND created_by_id = ?", ticketID, userID).First(&tkt).Error; err != nil {
		return nil, nil, err
	}
	if tkt.Status == models.StatusClosed {
		return nil, nil, errors.New("tiket ini sudah ditutup dan tidak bisa dibalas")
	}

	isDelivered := false
	isRead := false
	if len(deliveryStatus) > 0 {
		isDelivered = deliveryStatus[0]
	}
	if len(deliveryStatus) > 1 {
		isRead = deliveryStatus[1]
	}
	var readAt *time.Time
	if isRead {
		nowRead := time.Now()
		readAt = &nowRead
	}

	reply = &models.TicketReply{
		TicketID:    tkt.ID,
		UserID:      userID,
		Message:     message,
		IsDelivered: isDelivered,
		IsRead:      isRead,
		ReadAt:      readAt,
	}
	if err := config.DB.Create(reply).Error; err != nil {
		logging.TicketChat.Error("Failed to save ticket reply",
			"ticket_id", tkt.ID,
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, nil, err
	}

	logging.TicketChat.Info("Ticket reply added",
		"ticket_id", tkt.ID,
		"reply_id", reply.ID,
		"user_id", userID,
		"message_len", len(message),
		"attachments_count", len(attachments),
	)

	for i := range attachments {
		attachments[i].TicketID = tkt.ID
		attachments[i].ReplyID = &reply.ID
		ticketNumClean := strings.TrimSpace(tkt.GetTicketNumber())
		ticketNumClean = strings.ReplaceAll(ticketNumClean, " ", "")
		base := filepath.Base(attachments[i].FilePath)
		if attachments[i].FilePath != "" && !strings.HasPrefix(base, ticketNumClean+"-") {
			oldPath := filepath.FromSlash(attachments[i].FilePath)
			timestamp := time.Now().Unix()
			newName := utils.GenerateTicketAttachmentFileName(ticketNumClean, timestamp, i+1, len(attachments), attachments[i].FileName)
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			if _, err := os.Stat(newPath); err == nil {
				ext := filepath.Ext(newName)
				baseName := strings.TrimSuffix(newName, ext)
				counter := 1
				for {
					candidateName := fmt.Sprintf("%s_%d%s", baseName, counter, ext)
					candPath := filepath.Join(filepath.Dir(oldPath), candidateName)
					if _, err := os.Stat(candPath); os.IsNotExist(err) {
						newName = candidateName
						newPath = candPath
						break
					}
					counter++
				}
			}
			if _, err := os.Stat(oldPath); err == nil {
				_ = os.Rename(oldPath, newPath)
			}
			attachments[i].FilePath = filepath.ToSlash(newPath)
		}
		if err := config.DB.Create(&attachments[i]).Error; err != nil {
			logging.TicketAttachments.Error("Failed to save reply attachment",
				"ticket_id", tkt.ID,
				"reply_id", reply.ID,
				"file_name", attachments[i].FileName,
				"error", err.Error(),
			)
			utils.CleanupAttachments(attachments[i : i+1])
		} else {
			logging.TicketAttachments.Info("Reply attachment saved",
				"ticket_id", tkt.ID,
				"reply_id", reply.ID,
				"attachment_id", attachments[i].ID,
				"file_name", attachments[i].FileName,
				"file_size", attachments[i].FileSize,
				"is_pdf", attachments[i].IsPDF(),
			)
		}
	}

	config.DB.Preload("User").Preload("Attachments").First(reply, reply.ID)
	config.DB.Model(&tkt).Update("updated_at", time.Now())

	var user models.User
	config.DB.First(&user, userID)
	if !user.IsStaff && tkt.AssignedToID != nil {
		go func() {
			var check models.Ticket
			if err := config.DB.First(&check, tkt.ID).Error; err != nil {
				return
			}
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
	err = config.DB.Create(&newRating).Error
	if err != nil {
		logging.TicketRatings.Error("Failed to save ticket rating via token", "ticket_id", ticketID, "error", err.Error())
	} else {
		logging.TicketRatings.Info("Ticket rating saved via token", "ticket_id", ticketID, "rating", rating, "user_id", claims.UserID)
	}
	return err
}

// SubmitRatingForUser saves rating directly for an authenticated user.
func (s *TicketService) SubmitRatingForUser(ticketID int, userID uint, rating int, comment string) error {
	var ticket models.Ticket
	if err := config.DB.Where("id = ? AND created_by_id = ? AND status = ?", ticketID, userID, models.StatusClosed).First(&ticket).Error; err != nil {
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
		RatedByID: userID,
		RatedAt:   time.Now(),
	}
	err := config.DB.Create(&newRating).Error
	if err != nil {
		logging.TicketRatings.Error("Failed to save ticket rating for user", "ticket_id", ticketID, "user_id", userID, "error", err.Error())
	} else {
		logging.TicketRatings.Info("Ticket rating saved for user", "ticket_id", ticketID, "user_id", userID, "rating", rating)
	}
	return err
}

// ResolveSLAPolicy delegates to models.ResolveSLAPolicy using config.DB.
func (s *TicketService) ResolveSLAPolicy(companyID, departmentID *uint, priority ...models.TicketPriority) (*models.SLAPolicy, time.Duration, time.Duration) {
	return models.ResolveSLAPolicy(config.DB, companyID, departmentID, priority...)
}
