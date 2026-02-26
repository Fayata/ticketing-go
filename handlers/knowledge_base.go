package handlers

import (
	"encoding/json"
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
	categorySlug := strings.TrimSpace(r.URL.Query().Get("category"))

	unreadCount, _ := models.GetUnreadCount(config.DB, user.ID)
	viewAll := strings.TrimSpace(r.URL.Query().Get("view")) == "all"
	var allArticles []models.KBArticle
	var searchResults []models.KBArticle
	showSearchResults := false
	var categoryID *uint
	if categorySlug != "" {
		id := h.kbService.GetCategoryIDBySlug(categorySlug)
		if id != 0 {
			categoryID = &id
		}
	}
	if q != "" || (categoryID != nil && *categoryID != 0) {
		showSearchResults = true
		searchResults = h.kbService.SearchKBArticles(q, categoryID, 200)
	} else if viewAll {
		allArticles = h.kbService.GetKBAllArticles(500)
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":                "Knowledge Base — Ticketing",
		"page_title":           "Knowledge Base",
		"page_subtitle":        "Temukan jawaban untuk pertanyaan Anda",
		"nav_active":           "kb",
		"template_name":        "tickets/knowledge_base",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"unread_count":         unreadCount,
		"categories":           kbData.Categories,
		"popular_articles":     kbData.PopularArticles,
		"recent_articles":      kbData.RecentArticles,
		"total_articles":       kbData.TotalArticles,
		"total_categories":     kbData.TotalCategories,
		"search_query":         q,
		"view_all":             viewAll,
		"all_articles":         allArticles,
		"show_search_results":  showSearchResults,
		"search_results":       searchResults,
		"selected_category":   categorySlug,
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
	var catID *uint
	if article.CategoryID != 0 {
		catID = &article.CategoryID
	}
	relatedArticles := h.kbService.GetRelatedArticles(article.ID, catID, 6)
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
		"related_articles":     relatedArticles,
	})

	RenderTemplate(w, "tickets/kb_article", data)
}

// RecordKBArticleView increments view count when user has scrolled to end of article (API).
func (h *DashboardHandler) RecordKBArticleView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ArticleID uint `json:"article_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idStr := r.URL.Query().Get("id")
		if idStr != "" {
			if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
				body.ArticleID = uint(id)
			}
		}
	}
	if body.ArticleID == 0 {
		http.Error(w, "article_id required", http.StatusBadRequest)
		return
	}
	if err := h.kbService.RecordArticleView(body.ArticleID); err != nil {
		http.Error(w, "failed to record view", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}
