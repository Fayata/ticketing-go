package services

import (
	"errors"
	"fmt"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type AuthService struct {
	cfg          *config.Config
	emailService *utils.EmailService
	jwtService   *utils.JWTService
}

func NewAuthService(cfg *config.Config, emailService *utils.EmailService, jwtService *utils.JWTService) *AuthService {
	return &AuthService{
		cfg:          cfg,
		emailService: emailService,
		jwtService:   jwtService,
	}
}

// RegisterUser: Buat user, generate token, kirim email
func (s *AuthService) RegisterUser(username, email, password string) error {
	// 1. Cek duplikasi (Logic pindahan dari handler)
	var existingUser models.User
	if err := config.DB.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		return errors.New("username atau email sudah digunakan")
	}

	hashedPassword, _ := utils.HashPassword(password)

	user := models.User{
		Username:   username,
		Email:      email,
		Password:   hashedPassword,
		IsActive:   true,
		IsVerified: false, // Default false
	}

	if err := config.DB.Create(&user).Error; err != nil {
		return err
	}

	// 2. Generate Token Verifikasi
	token, err := s.jwtService.GenerateToken(user.ID, "verify_email", 24*time.Hour)
	if err != nil {
		return err
	}

	// 3. Kirim Email (Perlu update EmailService utk support ini)
	link := fmt.Sprintf("%s/verify-email?token=%s", s.cfg.BaseURL, token)
	go s.emailService.SendMail(email, "Verifikasi Email", "Klik link ini untuk verifikasi: "+link)

	return nil
}

// VerifyEmail: Validasi token dan update status user
func (s *AuthService) VerifyEmail(token string) error {
	claims, err := s.jwtService.ValidateToken(token)
	if err != nil {
		return errors.New("token tidak valid atau kadaluarsa")
	}

	if claims.Purpose != "verify_email" {
		return errors.New("token tidak sesuai")
	}

	var user models.User
	if err := config.DB.First(&user, claims.UserID).Error; err != nil {
		return errors.New("user tidak ditemukan")
	}

	user.IsVerified = true
	config.DB.Save(&user)
	return nil
}

// Authenticate: Cek login
func (s *AuthService) Authenticate(username, password string) (*models.User, error) {
	var user models.User
	if err := config.DB.Preload("Groups").Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		return nil, errors.New("username atau password salah")
	}

	if !utils.CheckPasswordHash(password, user.Password) {
		return nil, errors.New("username atau password salah")
	}

	// Tambahan: Cek Verifikasi Email
	if !user.IsVerified {
		return nil, errors.New("silakan verifikasi email anda terlebih dahulu")
	}

	return &user, nil
}
