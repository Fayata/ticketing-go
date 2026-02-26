package services

import (
	"ticketing/config"
	"ticketing/models"
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

// GetKBArticleByID loads published article by ID and increments views. Returns nil if not found.
func (s *KBService) GetKBArticleByID(id uint) (*models.KBArticle, error) {
	var article models.KBArticle
	if err := config.DB.Preload("Category").Where("id = ? AND published = ? AND deleted_at IS NULL", id, true).First(&article).Error; err != nil {
		return nil, err
	}
	config.DB.Model(&article).Update("views", article.Views+1)
	article.Views++
	return &article, nil
}
