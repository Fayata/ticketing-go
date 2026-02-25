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
	unreadCount, _ := models.GetUnreadCount(config.DB, user.ID)

	// Ambil kategori
	var categories []models.KBCategory
	config.DB.Where("deleted_at IS NULL").Order("sort_order ASC, name ASC").Find(&categories)

	// Hitung jumlah artikel per kategori untuk ditampilkan di card
	type catWithCount struct {
		Category     models.KBCategory
		ArticleCount int
	}
	var categoriesWithCount []catWithCount
	for _, c := range categories {
		var count int64
		config.DB.Model(&models.KBArticle{}).Where("category_id = ? AND published = ? AND deleted_at IS NULL", c.ID, true).Count(&count)
		categoriesWithCount = append(categoriesWithCount, catWithCount{Category: c, ArticleCount: int(count)})
	}

	// Artikel populer (urut views desc)
	var popularArticles []models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("views DESC").Limit(6).Find(&popularArticles)

	// Artikel terbaru (updated_at desc)
	var recentArticles []models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("updated_at DESC").Limit(5).Find(&recentArticles)

	// Total artikel & kategori untuk stats hero
	var totalArticles, totalCategories int64
	config.DB.Model(&models.KBArticle{}).Where("published = ? AND deleted_at IS NULL", true).Count(&totalArticles)
	config.DB.Model(&models.KBCategory{}).Where("deleted_at IS NULL").Count(&totalCategories)

	// Query pencarian (opsional)
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	data := AddBaseData(r, map[string]interface{}{
		"title":                "Knowledge Base — Ticketing",
		"page_title":           "Knowledge Base",
		"page_subtitle":        "Temukan jawaban untuk pertanyaan Anda",
		"nav_active":           "kb",
		"template_name":        "tickets/knowledge_base",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"unread_count":         unreadCount,
		"categories":           categoriesWithCount,
		"popular_articles":     popularArticles,
		"recent_articles":      recentArticles,
		"total_articles":       totalArticles,
		"total_categories":     totalCategories,
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
	unreadCount, _ := models.GetUnreadCount(config.DB, user.ID)

	var article models.KBArticle
	if err := config.DB.Preload("Category").Where("id = ? AND published = ? AND deleted_at IS NULL", uint(id), true).First(&article).Error; err != nil {
		http.Redirect(w, r, config.Path("/knowledge-base"), http.StatusSeeOther)
		return
	}
	// Increment views
	config.DB.Model(&article).Update("views", article.Views+1)
	article.Views++

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
