package main

import (
	"errors"

	"ticketing/config"
	"ticketing/internal/security"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

type AuthServiceWrapper struct {
	*services.AuthService
}

func NewAuthService(cfg *config.Config) *AuthServiceWrapper {
	emailService := utils.NewEmailService(cfg)
	jwtService := utils.NewJWTService(cfg)
	
	return &AuthServiceWrapper{
		AuthService: services.NewAuthService(cfg, emailService, jwtService),
	}
}

func (s *AuthServiceWrapper) RegisterUser(username, email, password string) error {
	// Add stronger password validation
	if security.ValidatePassword(password) != nil {
		return errors.New("password tidak memenuhi standar keamanan")
	}

	// Call underlying
	err := s.AuthService.RegisterUser(username, email, password)
	if err != nil {
		return err
	}

	// Override RegisterUser to set IsVerified: false (not true) so email verification is enforced
	config.DB.Model(&models.User{}).Where("username = ?", username).Update("is_verified", false)

	security.LogAudit(security.AuditEvent{Action: "REGISTER_SUCCESS", Details: "User registered successfully: "+username})
	return nil
}

func (s *AuthServiceWrapper) ResetPassword(token, newPassword string) error {
	if security.ValidatePassword(newPassword) != nil {
		return errors.New("password tidak memenuhi standar keamanan")
	}

	err := s.AuthService.ResetPassword(token, newPassword)
	if err == nil {
		security.LogAudit(security.AuditEvent{Action: "PASSWORD_RESET_SUCCESS", Details: "Password reset successful"})
	} else {
		security.LogAudit(security.AuditEvent{Action: "PASSWORD_RESET_FAILED", Details: "Password reset failed: "+err.Error()})
	}
	return err
}
