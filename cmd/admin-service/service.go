package main

import (
	"log"
	"ticketing/services"
)

// AdminDashboardServiceWrapper wraps the admin dashboard service to add specialized logging.
type AdminDashboardServiceWrapper struct {
	*services.AdminDashboardService
}

// NewAdminDashboardServiceWrapper creates a new wrapper for the dashboard service.
func NewAdminDashboardServiceWrapper(s *services.AdminDashboardService) *AdminDashboardServiceWrapper {
	return &AdminDashboardServiceWrapper{
		AdminDashboardService: s,
	}
}

// AdminSearchServiceWrapper wraps the admin search service to add logging and tracking.
type AdminSearchServiceWrapper struct {
	*services.AdminSearchService
}

// NewAdminSearchServiceWrapper creates a new wrapper for the search service.
func NewAdminSearchServiceWrapper(s *services.AdminSearchService) *AdminSearchServiceWrapper {
	return &AdminSearchServiceWrapper{
		AdminSearchService: s,
	}
}

// LogSensitiveOperation logs any sensitive operations performed by the admin services.
func LogSensitiveOperation(operation string, details string) {
	log.Printf("[SECURITY] Sensitive operation performed: %s. Details: %s", operation, details)
}
