package main

import (
	"log"
	"ticketing/models"
)

// NotificationServiceInterface is a mock interface that should match ticketing/services.NotificationService
type NotificationServiceInterface interface {
	CreateNotification(userID uint, title, message, url, icon string) error
	GetNotifications(userID uint, limit, offset int) ([]models.Notification, int64, error)
	MarkAsRead(notificationID uint, userID uint) error
	MarkAllAsRead(userID uint) error
	GetUnreadCount(userID uint) (int64, error)
}

// EnhancedNotificationService wraps the existing notification service to add logging
type EnhancedNotificationService struct {
	baseService NotificationServiceInterface
}

// NewEnhancedNotificationService creates a new instance of EnhancedNotificationService
func NewEnhancedNotificationService(baseService interface{}) *EnhancedNotificationService {
	return &EnhancedNotificationService{
		baseService: baseService.(NotificationServiceInterface),
	}
}

// CreateNotification logs the event
func (s *EnhancedNotificationService) CreateNotification(userID uint, title, message, url, icon string) error {
	err := s.baseService.CreateNotification(userID, title, message, url, icon)
	if err == nil {
		log.Printf("Event: Notification created [UserID: %d, Title: %s]", userID, title)
	}
	return err
}

// GetNotifications is passed through
func (s *EnhancedNotificationService) GetNotifications(userID uint, limit, offset int) ([]models.Notification, int64, error) {
	return s.baseService.GetNotifications(userID, limit, offset)
}

// MarkAsRead logs the event
func (s *EnhancedNotificationService) MarkAsRead(notificationID uint, userID uint) error {
	err := s.baseService.MarkAsRead(notificationID, userID)
	if err == nil {
		log.Printf("Event: Notification marked as read [NotificationID: %d, UserID: %d]", notificationID, userID)
	}
	return err
}

// MarkAllAsRead logs the event
func (s *EnhancedNotificationService) MarkAllAsRead(userID uint) error {
	err := s.baseService.MarkAllAsRead(userID)
	if err == nil {
		log.Printf("Event: All notifications marked as read [UserID: %d]", userID)
	}
	return err
}

// GetUnreadCount is passed through
func (s *EnhancedNotificationService) GetUnreadCount(userID uint) (int64, error) {
	return s.baseService.GetUnreadCount(userID)
}
