package main

import (
	"errors"
	"log"

	"gorm.io/gorm"
	"ticketing/models"
)

// TicketServiceInterface is a mock interface that should match ticketing/services.TicketService
type TicketServiceInterface interface {
	CreateTicket(ticket *models.Ticket) error
	CloseTicket(ticketID uint, userID uint) error
	ClaimTicket(ticketID uint, staffID uint) error
	ReleaseTicket(ticketID uint, staffID uint) error
}

// EnhancedTicketService wraps the existing ticket service to add validation and logging
type EnhancedTicketService struct {
	baseService TicketServiceInterface
	db          *gorm.DB
}

// NewEnhancedTicketService creates a new instance of EnhancedTicketService
func NewEnhancedTicketService(baseService interface{}, db *gorm.DB) *EnhancedTicketService {
	// We cast to interface for compatibility, or adjust according to actual codebase
	return &EnhancedTicketService{
		baseService: baseService.(TicketServiceInterface),
		db:          db,
	}
}

// CreateTicket validates input and department before calling the base service
func (s *EnhancedTicketService) CreateTicket(ticket *models.Ticket) error {
	// Validate department
	var dept models.Department
	if err := s.db.First(&dept, ticket.DepartmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("invalid department ID")
		}
		return err
	}

	// Call base
	err := s.baseService.CreateTicket(ticket)
	if err == nil {
		log.Printf("Event: Ticket created [ID: %d, Title: %s, Dept: %d]", ticket.ID, ticket.Title, ticket.DepartmentID)
	}
	return err
}

// CloseTicket logs the close event
func (s *EnhancedTicketService) CloseTicket(ticketID uint, userID uint) error {
	err := s.baseService.CloseTicket(ticketID, userID)
	if err == nil {
		log.Printf("Event: Ticket closed [ID: %d, UserID: %d]", ticketID, userID)
	}
	return err
}

// ClaimTicket logs the claim event
func (s *EnhancedTicketService) ClaimTicket(ticketID uint, staffID uint) error {
	err := s.baseService.ClaimTicket(ticketID, staffID)
	if err == nil {
		log.Printf("Event: Ticket claimed [ID: %d, StaffID: %d]", ticketID, staffID)
	}
	return err
}

// ReleaseTicket logs the release event
func (s *EnhancedTicketService) ReleaseTicket(ticketID uint, staffID uint) error {
	err := s.baseService.ReleaseTicket(ticketID, staffID)
	if err == nil {
		log.Printf("Event: Ticket released [ID: %d, StaffID: %d]", ticketID, staffID)
	}
	return err
}
