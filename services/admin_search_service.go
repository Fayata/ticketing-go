package services

import (
	"log"
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
	query := config.DB.Preload("Department").Preload("CreatedBy")

	// Apply Department filter
	if filters.Department != "" {
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
		kw := "%" + filters.Keyword + "%"
		query = query.Where("tickets.title ILIKE ? OR tickets.description ILIKE ?", kw, kw)
	}

	// Fetch results
	err := query.Order("tickets.created_at DESC").Limit(50).Find(&tickets).Error
	if err != nil {
		return tickets, err
	}

	// Fallback: if keyword produced 0 results but other filters are empty,
	// retry without keyword to show all tickets instead of empty page
	if len(tickets) == 0 && filters.Keyword != "" && filters.Department == "" && filters.Status == "" && filters.Priority == "" {
		log.Printf("[Search] Keyword '%s' returned 0 results, retrying without keyword filter", filters.Keyword)
		err = config.DB.Preload("Department").Preload("CreatedBy").
			Order("tickets.created_at DESC").Limit(50).Find(&tickets).Error
	}

	return tickets, err
}
