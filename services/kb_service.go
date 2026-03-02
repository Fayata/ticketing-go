package services

import (
	"ticketing/config"
	"ticketing/models"

	"gorm.io/gorm"
)

type KBService struct{}

func NewKBService() *KBService {
	return &KBService{}
}

// CatWithCount for KB list page.
type CatWithCount struct {
	Category     models.KBCategory
	ArticleCount int
}

// KBPageData for knowledge base list page.
type KBPageData struct {
	Categories      []CatWithCount
	PopularArticles []models.KBArticle
	RecentArticles  []models.KBArticle
	TotalArticles   int64
	TotalCategories int64
}

// GetKBPageData returns data for the KB list page.
func (s *KBService) GetKBPageData() (*KBPageData, error) {
	var categories []models.KBCategory
	config.DB.Where("deleted_at IS NULL").Order("sort_order ASC, name ASC").Find(&categories)

	var categoriesWithCount []CatWithCount
	for _, c := range categories {
		var count int64
		config.DB.Model(&models.KBArticle{}).Where("category_id = ? AND published = ? AND deleted_at IS NULL", c.ID, true).Count(&count)
		categoriesWithCount = append(categoriesWithCount, CatWithCount{Category: c, ArticleCount: int(count)})
	}

	var popularArticles []models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("views DESC").Limit(6).Find(&popularArticles)

	// Baru Diperbarui: urutkan hanya berdasarkan updated_at (kapan artikel diedit), bukan dari penambahan view
	var recentArticles []models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("updated_at DESC").Limit(5).Find(&recentArticles)

	var totalArticles, totalCategories int64
	config.DB.Model(&models.KBArticle{}).Where("published = ? AND deleted_at IS NULL", true).Count(&totalArticles)
	config.DB.Model(&models.KBCategory{}).Where("deleted_at IS NULL").Count(&totalCategories)

	return &KBPageData{
		Categories:      categoriesWithCount,
		PopularArticles: popularArticles,
		RecentArticles:  recentArticles,
		TotalArticles:   totalArticles,
		TotalCategories: totalCategories,
	}, nil
}

// GetKBArticleByID loads published article by ID (does not increment views). Returns nil if not found.
func (s *KBService) GetKBArticleByID(id uint) (*models.KBArticle, error) {
	var article models.KBArticle
	if err := config.DB.Preload("Category").Where("id = ? AND published = ? AND deleted_at IS NULL", id, true).First(&article).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

// RecordArticleView increments view count only; tidak mengubah updated_at agar "Baru Diperbarui" tetap berdasarkan edit artikel.
func (s *KBService) RecordArticleView(articleID uint) error {
	return config.DB.Model(&models.KBArticle{}).Where("id = ? AND published = ? AND deleted_at IS NULL", articleID, true).
		UpdateColumn("views", gorm.Expr("views + 1")).Error
}

// GetRelatedArticles returns articles from the same category (or popular) excluding the given article.
func (s *KBService) GetRelatedArticles(articleID uint, categoryID *uint, limit int) []models.KBArticle {
	if limit <= 0 {
		limit = 6
	}
	var list []models.KBArticle
	q := config.DB.Preload("Category").Where("id != ? AND published = ? AND deleted_at IS NULL", articleID, true)
	if categoryID != nil && *categoryID != 0 {
		q = q.Where("category_id = ?", *categoryID).Order("updated_at DESC")
	} else {
		q = q.Order("views DESC")
	}
	q.Limit(limit).Find(&list)
	return list
}

// GetKBAllArticles returns all published articles (for "Lihat Semua").
func (s *KBService) GetKBAllArticles(limit int) []models.KBArticle {
	if limit <= 0 {
		limit = 500
	}
	var list []models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("views DESC, updated_at DESC").Limit(limit).Find(&list)
	return list
}

// SearchKBArticles returns articles filtered by query (title/content) and optional category.
func (s *KBService) SearchKBArticles(query string, categoryID *uint, limit int) []models.KBArticle {
	if limit <= 0 {
		limit = 200
	}
	q := config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true)
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("title ILIKE ? OR content ILIKE ? OR slug ILIKE ?", like, like, like)
	}
	if categoryID != nil && *categoryID != 0 {
		q = q.Where("category_id = ?", *categoryID)
	}
	var list []models.KBArticle
	q.Order("views DESC, updated_at DESC").Limit(limit).Find(&list)
	return list
}

// GetCategoryIDBySlug returns category ID for slug, or 0 if not found.
func (s *KBService) GetCategoryIDBySlug(slug string) uint {
	if slug == "" {
		return 0
	}
	var c models.KBCategory
	if err := config.DB.Where("slug = ? AND deleted_at IS NULL", slug).First(&c).Error; err != nil {
		return 0
	}
	return c.ID
}
