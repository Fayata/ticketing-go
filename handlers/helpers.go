package handlers

import (
	"net/http"

	"ticketing/utils"
)

// RenderTemplate delegates to utils (single source for template loading and rendering).
func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	utils.RenderTemplate(w, tmpl, data)
}

// GetUserFromContext returns user from request context (set by auth middleware).
func GetUserFromContext(r *http.Request) interface{} {
	return utils.GetUserFromContext(r)
}

// GetActiveTicketsCount returns active tickets count from context.
func GetActiveTicketsCount(r *http.Request) interface{} {
	return utils.GetActiveTicketsCount(r)
}

// AddBaseData merges base template data (user, active_tickets_count, unread_count, title).
func AddBaseData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	return utils.AddBaseData(r, data)
}
