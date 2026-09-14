package utils

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/middleware"
	"ticketing/models"
)

var templates *template.Template
var wibLocation = time.FixedZone("WIB", 7*3600)

// InitTemplates memuat semua template HTML dan mendaftarkan helper (date, eq, len, dll) untuk dipakai di template.
func InitTemplates() {
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
				if v.IsZero() {
					return ""
				}
				return v.In(wibLocation).Format("02 Jan 2006, 15:04")
			case *time.Time:
				if v == nil || v.IsZero() {
					return ""
				}
				return v.In(wibLocation).Format("02 Jan 2006, 15:04")
			}
			return ""
		},
		"dateShort": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case time.Time:
				if v.IsZero() {
					return ""
				}
				return v.In(wibLocation).Format("02 Jan 2006")
			case *time.Time:
				if v == nil || v.IsZero() {
					return ""
				}
				return v.In(wibLocation).Format("02 Jan 2006")
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

		"add": func(a, b interface{}) int {
			toInt := func(x interface{}) int {
				switch v := x.(type) {
				case int:
					return v
				case int64:
					return int(v)
				case float64:
					return int(v)
				default:
					return 0
				}
			}
			return toInt(a) + toInt(b)
		},
		"eq": func(a, b interface{}) bool {
			if a == b {
				return true
			}
			if a == nil || b == nil {
				return false
			}
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		},
		"len": func(arr interface{}) int {
			if arr == nil {
				return 0
			}
			val := reflect.ValueOf(arr)
			switch val.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map, reflect.String, reflect.Chan:
				return val.Len()
			default:
				return 0
			}
		},

		"linebreaks": func(val interface{}) template.HTML {
			if val == nil {
				return ""
			}
			s := fmt.Sprint(val)
			// [Security] Escape HTML entities FIRST to prevent XSS, then add <br>
			s = template.HTMLEscapeString(s)
			s = strings.ReplaceAll(s, "&#13;&#10;", "<br>")
			s = strings.ReplaceAll(s, "&#10;", "<br>")
			s = strings.ReplaceAll(s, "\r\n", "<br>")
			s = strings.ReplaceAll(s, "\n", "<br>")
			return template.HTML(s)
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
		"getFullName": func(user interface{}) string {
			if user == nil {
				return "User"
			}

			// Handle pointer to User
			if u, ok := user.(*models.User); ok {
				if u.FirstName != "" || u.LastName != "" {
					return strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return u.Username
			}
			// Handle struct User
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
	}

	tmpl := template.New("").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "tickets", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "admin", "*.html")))

	templates = tmpl
	logging.HTTPTemplate.Info("Templates loaded successfully with helper functions")
}

// TruncateString memotong string sampai maxLen karakter; sisanya diganti "...".
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// RenderTemplate merender template HTML dengan data ke response.
func RenderTemplate(w http.ResponseWriter, tmplName string, data interface{}) {
	if templates == nil {
		logging.HTTPTemplate.Error("Templates not initialized", "template", tmplName)
		http.Error(w, "Templates not initialized", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	start := time.Now()
	err := templates.ExecuteTemplate(w, tmplName, data)
	duration := time.Since(start)

	if err != nil {
		logging.HTTPTemplate.Error("Template render error",
			"template", tmplName,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logging.HTTPTemplate.Debug("Template rendered successfully",
		"template", tmplName,
		"duration_ms", duration.Milliseconds(),
	)
}

// GetUserFromContext mengembalikan user dari context (diisi middleware auth).
func GetUserFromContext(r *http.Request) interface{} {
	return r.Context().Value(middleware.UserKey)
}

// GetActiveTicketsCount mengembalikan jumlah tiket aktif user dari context.
func GetActiveTicketsCount(r *http.Request) interface{} {
	count := r.Context().Value(middleware.ActiveTicketsCountKey)
	if count == nil {
		return 0
	}
	return count
}

// AddBaseData menggabungkan data dasar (user, title, unread_count, static_base) ke map data untuk template.
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

	if count := r.Context().Value(middleware.ActiveTicketsCountKey); count != nil {
		data["active_tickets_count"] = count
	} else {
		data["active_tickets_count"] = 0
	}

	if user := GetUserFromContext(r); user != nil {
		if u, ok := user.(*models.User); ok {
			unreadCount, _ := models.GetUnreadCount(config.DB, u.ID)
			data["unread_count"] = unreadCount
			if u.IsStaff {
				var deptID uint
				if u.DepartmentID != nil {
					deptID = *u.DepartmentID
				}
				unreadReplies, _ := models.GetUnreadRepliesCountForStaff(config.DB, u.ID, deptID)
				data["unread_replies_count"] = unreadReplies
			} else {
				unreadReplies, _ := models.GetUnreadRepliesCount(config.DB, u.ID)
				data["unread_replies_count"] = unreadReplies
			}
		}
	} else {
		data["unread_count"] = 0
		data["unread_replies_count"] = 0
	}
	if data["unread_replies_count"] == nil {
		data["unread_replies_count"] = 0
	}
	if data["static_base"] == nil {
		data["static_base"] = config.Path("/static")
	}

	return data
}
