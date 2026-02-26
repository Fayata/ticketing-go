package services

import (
	"ticketing/config"
	"ticketing/models"
)

type DashboardService struct{}

func NewDashboardService() *DashboardService {
	return &DashboardService{}
}

// DashboardData holds data for user dashboard page.
type DashboardData struct {
	WaitingCount     int64
	InProgressCount  int64
	ClosedCount      int64
	TotalCount       int64
	RecentTickets    []*models.Ticket
	PopularArticles  []*models.KBArticle
	UnreadCount      int64
}

// GetDashboardData returns counts and lists for the portal user dashboard.
func (s *DashboardService) GetDashboardData(userID uint) (*DashboardData, error) {
	var waitingCount, inProgressCount, closedCount, totalCount int64
	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", userID, models.StatusWaiting).
		Count(&waitingCount)
	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", userID, models.StatusInProgress).
		Count(&inProgressCount)
	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", userID, models.StatusClosed).
		Count(&closedCount)
	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ?", userID).
		Count(&totalCount)

	var recentTickets []*models.Ticket
	config.DB.Preload("Department").Preload("Replies").
		Where("created_by_id = ?", userID).
		Order("created_at DESC").Limit(5).
		Find(&recentTickets)

	unreadCount, _ := models.GetUnreadCount(config.DB, userID)

	var popularArticles []*models.KBArticle
	config.DB.Preload("Category").Where("published = ? AND deleted_at IS NULL", true).
		Order("views DESC").Limit(6).Find(&popularArticles)
	if popularArticles == nil {
		popularArticles = []*models.KBArticle{}
	}

	return &DashboardData{
		WaitingCount:    waitingCount,
		InProgressCount: inProgressCount,
		ClosedCount:     closedCount,
		TotalCount:      totalCount,
		RecentTickets:   recentTickets,
		PopularArticles: popularArticles,
		UnreadCount:     unreadCount,
	}, nil
}
