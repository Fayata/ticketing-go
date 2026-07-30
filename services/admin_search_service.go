package services

import (
	"strings"

	"ticketing/config"
	"ticketing/models"
)

type AdminSearchService struct{}

func NewAdminSearchService() *AdminSearchService {
	return &AdminSearchService{}
}

// SearchTickets queries the database based on AI generated filters.
func (s *AdminSearchService) SearchTickets(filters AIFilters) ([]models.Ticket, error) {
	var tickets []models.Ticket
	query := config.DB.Preload("Department").Preload("User")

	// Apply Department filter
	if filters.Department != "" {
		// Try exact match first, or LIKE
		query = query.Joins("JOIN departments ON departments.id = tickets.department_id").
			Where("departments.name ILIKE ?", "%"+filters.Department+"%")
	}

	// Apply Status filter
	if filters.Status != "" {
		query = query.Where("tickets.status = ?", strings.ToUpper(filters.Status))
	}

	// Apply Priority filter
	if filters.Priority != "" {
		query = query.Where("tickets.priority = ?", strings.ToUpper(filters.Priority))
	}

	// Apply Keyword filter
	if filters.Keyword != "" {
		// Basic SQLi prevention - though GORM parameterization handles it
		kw := "%" + filters.Keyword + "%"
		query = query.Where("tickets.title ILIKE ? OR tickets.description ILIKE ?", kw, kw)
	}

	// Fetch results
	err := query.Order("tickets.created_at DESC").Limit(50).Find(&tickets).Error
	return tickets, err
}
