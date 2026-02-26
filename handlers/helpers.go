package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/middleware"
	"ticketing/models"
)

var templates *template.Template

func init() {
	// Fungsi untuk template (slice, date, status, dll.)
	funcMap := template.FuncMap{
		"slice": func(s string, start, end int) string {
			if start < 0 || end > len(s) || start > end {
				return s
			}
			return s[start:end]
		},
		"upper": strings.ToUpper,
		"date": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case time.Time:
				return v.Format("02 Jan 2006, 15:04")
			case *time.Time:
				if v == nil {
					return ""
				}
				return v.Format("02 Jan 2006, 15:04")
			}
			return ""
		},
		"dateShort": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case time.Time:
				return v.Format("02 Jan 2006")
			case *time.Time:
				if v == nil {
					return ""
				}
				return v.Format("02 Jan 2006")
			}
			return ""
		},
		"timeSince": func(t time.Time) string {
			now := time.Now()
			diff := now.Sub(t)
			days := int(diff.Hours() / 24)
			hours := int(diff.Hours())
			minutes := int(diff.Minutes())
			if days > 0 {
				return fmt.Sprintf("%d hari", days)
			}
			if hours > 0 {
				return fmt.Sprintf("%d jam", hours)
			}
			if minutes > 0 {
				return fmt.Sprintf("%d menit", minutes)
			}
			return "Baru saja"
		},
		"getStatusClass": func(status interface{}) string {
			s := fmt.Sprintf("%v", status)
			s = strings.ToUpper(s)
			switch s {
			case "WAITING", "OPEN":
				return "open"
			case "IN_PROGRESS":
				return "in-progress"
			case "CLOSED", "RESOLVED":
				return "closed"
			default:
				return "closed"
			}
		},
		"getPriorityClass": func(priority interface{}) string {
			p := fmt.Sprintf("%v", priority)
			p = strings.ToUpper(p)
			switch p {
			case "HIGH":
				return "high"
			case "MEDIUM":
				return "medium"
			case "LOW":
				return "low"
			default:
				return "low"
			}
		},
		"eq": func(a, b interface{}) bool {
			return a == b
		},
		"len": func(arr interface{}) int {
			if arr == nil {
				return 0
			}
			switch v := arr.(type) {
			case []interface{}:
				return len(v)
			case []*models.Ticket:
				return len(v)
			case []models.Ticket:
				return len(v)
			case []models.TicketReply:
				return len(v)
			case []*models.TicketReply:
				return len(v)
			case []models.Department:
				return len(v)
			case string:
				return len(v)
			}
			return 0
		},
		"linebreaks": func(val interface{}) template.HTML {
			if val == nil {
				return ""
			}
			s := fmt.Sprint(val)
			s = strings.ReplaceAll(s, "\r\n", "<br>")
			s = strings.ReplaceAll(s, "\n", "<br>")
			return template.HTML(s)
		},
		"getFullName": func(user interface{}) string {
			if user == nil {
				return "User"
			}
			if u, ok := user.(*models.User); ok {
				if u.FirstName != "" || u.LastName != "" {
					return strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return u.Username
			}
			if u, ok := user.(models.User); ok {
				if u.FirstName != "" || u.LastName != "" {
					return strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return u.Username
			}
			return "User"
		},
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"add": func(a, b int) int { return a + b },
	}

	// Load templates with functions
	tmpl := template.New("").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "tickets", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "admin", "*.html")))

	templates = tmpl
}

// RenderTemplate renders HTML template
func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmpl, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	// Only write to ResponseWriter after template renders successfully to avoid
	// double WriteHeader on partial renders.
	_, _ = buf.WriteTo(w)
}

// GetUserFromContext gets user from request context
func GetUserFromContext(r *http.Request) interface{} {
	return r.Context().Value(middleware.UserKey)
}

// GetActiveTicketsCount gets active tickets count from context
func GetActiveTicketsCount(r *http.Request) interface{} {
	count := r.Context().Value(middleware.ActiveTicketsCountKey)
	if count == nil {
		return 0
	}
	return count
}

// AddBaseData adds base data to template data
func AddBaseData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}

	if data["title"] == nil {
		data["title"] = "Portal Ticketing"
	}

	if user := GetUserFromContext(r); user != nil {
		data["user"] = user
	}

	if count := GetActiveTicketsCount(r); count != nil {
		data["active_tickets_count"] = count
	} else {
		data["active_tickets_count"] = 0
	}

	// Get unread notification count if user exists
	if user := GetUserFromContext(r); user != nil {
		if u, ok := user.(*models.User); ok {
			unreadCount, _ := models.GetUnreadCount(config.DB, u.ID)
			data["unread_count"] = unreadCount
		}
	} else {
		data["unread_count"] = 0
	}

	return data
}
