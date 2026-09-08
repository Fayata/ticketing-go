package services

import (
	"errors"
	"fmt"
	"log"
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

// validatePassword memvalidasi kekuatan password.
func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password minimal 8 karakter")
	}
	hasLetter := false
	hasDigit := false
	for _, c := range password {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasLetter = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("password harus mengandung huruf dan angka")
	}
	return nil
}

// validateUsername memvalidasi format dan panjang username.
func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 50 {
		return errors.New("username harus 3-50 karakter")
	}
	return nil
}

// RegisterUser: Buat user, assign group, generate token, kirim email
func (s *AuthService) RegisterUser(username, email, password string) error {
	// [Security] Validasi input
	if err := validateUsername(username); err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	log.Printf("[Security][Auth] Registration attempt for username=%q email=%q", username, email)

	// 1. Cek duplikasi
	var existingUser models.User
	if err := config.DB.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		return errors.New("username atau email sudah digunakan")
	}

	hashedPassword, _ := utils.HashPassword(password)

	// 2. Create User
	emailPtr := &email
	user := models.User{
		Username:   username,
		Email:      emailPtr,
		Password:   hashedPassword,
		IsActive:   true,
		IsVerified: false, // [Security] User must verify email before login
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
	if err := config.DB.Preload("Groups").
		Where("username = ? OR (email IS NOT NULL AND email = ?)", username, username).
		First(&user).Error; err != nil {
		log.Printf("[Security][Auth] Failed login attempt: user=%q not found", username)
		return nil, errors.New("username atau password salah")
	}

	if !utils.CheckPasswordHash(password, user.Password) {
		log.Printf("[Security][Auth] Failed login attempt for user=%q from password mismatch", username)
		return nil, errors.New("username atau password salah")
	}

	// Akun tanpa email (dibuat admin): langsung izinkan login, nanti diarahkan set email
	// Akun dengan email tapi belum diverifikasi: blok
	if user.Email != nil && *user.Email != "" && !user.IsVerified {
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

	if err := validatePassword(newPassword); err != nil {
		return err
	}
	log.Printf("[Security][Auth] Password reset for user ID=%d", claims.UserID)

	hashedPassword, _ := utils.HashPassword(newPassword)
	user.Password = hashedPassword
	if err := config.DB.Save(&user).Error; err != nil {
		return err
	}

	return nil
}

