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

// RegisterUser: Buat user, assign group, generate token, kirim email
func (s *AuthService) RegisterUser(username, email, password string) error {
	// 1. Cek duplikasi
	var existingUser models.User
	if err := config.DB.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		return errors.New("username atau email sudah digunakan")
	}

	hashedPassword, _ := utils.HashPassword(password)

	// 2. Create User
	user := models.User{
		Username:   username,
		Email:      email,
		Password:   hashedPassword,
		IsActive:   true,
		IsVerified: true, // Auto-verified agar bisa langsung login
	}

	if err := config.DB.Create(&user).Error; err != nil {
		return err
	}
	var portalGroup models.Group
	if err := config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"}).Error; err != nil {
		return fmt.Errorf("gagal inisialisasi grup: %v", err)
	}

	if err := config.DB.Model(&user).Association("Groups").Append(&portalGroup); err != nil {
		return fmt.Errorf("gagal assign group: %v", err)
	}
	go func() {
		token, _ := s.jwtService.GenerateToken(user.ID, "verify_email", 24*time.Hour)
		link := fmt.Sprintf("%s/verify-email?token=%s", s.cfg.BaseURL, token)

		err := s.emailService.SendMail(email, "Verifikasi Email", "Klik link ini untuk verifikasi: "+link)
		if err != nil {
			fmt.Printf("⚠️ Email warning (background): %v\n", err)
		}
	}()

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
	// Preload Groups agar bisa dicek hak aksesnya di middleware
	if err := config.DB.Preload("Groups").Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		return nil, errors.New("username atau password salah")
	}

	if !utils.CheckPasswordHash(password, user.Password) {
		return nil, errors.New("username atau password salah")
	}

	if !user.IsVerified {
		return nil, errors.New("silakan verifikasi email anda terlebih dahulu")
	}

	return &user, nil
}

func (s *AuthService) RequestPasswordReset(email string) error {
	var user models.User
	// Cari user berdasarkan email
	if err := config.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return errors.New("email tidak ditemukan")
	}

	token, err := s.jwtService.GenerateToken(user.ID, "reset_password", 1*time.Hour)
	if err != nil {
		return err
	}

	link := fmt.Sprintf("%s/reset-password?token=%s", s.cfg.BaseURL, token)

	// Kirim Email (Async)
	go func() {
		subject := "Reset Password - Portal Ticketing"
		body := fmt.Sprintf("Halo %s,\n\nSeseorang meminta untuk mereset password akun Anda.\nKlik link di bawah ini untuk membuat password baru:\n\n%s\n\nLink ini akan kadaluarsa dalam 1 jam.\nJika ini bukan Anda, abaikan email ini.", user.Username, link)

		err := s.emailService.SendMail(email, subject, body)
		if err != nil {
			fmt.Printf("⚠️ Gagal kirim email reset: %v\n", err)
		}
	}()

	return nil
}

func (s *AuthService) ResetPassword(token, newPassword string) error {
	claims, err := s.jwtService.ValidateToken(token)
	if err != nil {
		return errors.New("link reset password sudah kadaluarsa atau tidak valid")
	}

	if claims.Purpose != "reset_password" {
		return errors.New("token tidak valid untuk reset password")
	}

	var user models.User
	if err := config.DB.First(&user, claims.UserID).Error; err != nil {
		return errors.New("user tidak ditemukan")
	}

	hashedPassword, _ := utils.HashPassword(newPassword)
	user.Password = hashedPassword
	if err := config.DB.Save(&user).Error; err != nil {
		return err
	}

	return nil
}
