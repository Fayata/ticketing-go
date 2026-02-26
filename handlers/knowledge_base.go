package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"ticketing/config"
	"ticketing/models"
)

// ShowKnowledgeBase menampilkan halaman Knowledge Base untuk user
func (h *DashboardHandler) ShowKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)
	activeTicketsCount := GetActiveTicketsCount(r)

	kbData, err := h.kbService.GetKBPageData()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))

	data := AddBaseData(r, map[string]interface{}{
		"title":                "Knowledge Base — Ticketing",
		"page_title":           "Knowledge Base",
		"page_subtitle":        "Temukan jawaban untuk pertanyaan Anda",
		"nav_active":           "kb",
		"template_name":        "tickets/knowledge_base",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"unread_count":         kbData.TotalArticles,
		"categories":           kbData.Categories,
		"popular_articles":     kbData.PopularArticles,
		"recent_articles":      kbData.RecentArticles,
		"total_articles":       kbData.TotalArticles,
		"total_categories":     kbData.TotalCategories,
		"search_query":         q,
	})

	RenderTemplate(w, "tickets/knowledge_base", data)
}

// ShowKBArticle menampilkan satu artikel KB (detail)
func (h *DashboardHandler) ShowKBArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/knowledge-base/article/")
	idStr = strings.Trim(idStr, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Redirect(w, r, config.Path("/knowledge-base"), http.StatusSeeOther)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	activeTicketsCount := GetActiveTicketsCount(r)

	article, err := h.kbService.GetKBArticleByID(uint(id))
	if err != nil {
		http.Redirect(w, r, config.Path("/knowledge-base"), http.StatusSeeOther)
		return
	}

	unreadCount, _ := models.GetUnreadCount(config.DB, user.ID)

	data := AddBaseData(r, map[string]interface{}{
		"title":                article.Title + " — Knowledge Base",
		"page_title":           article.Title,
		"page_subtitle":        "Knowledge Base",
		"nav_active":           "kb",
		"template_name":        "tickets/kb_article",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"unread_count":         unreadCount,
		"article":              article,
	})

	RenderTemplate(w, "tickets/kb_article", data)
}
