package services

import (
	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type SettingsService struct{}

func NewSettingsService() *SettingsService {
	return &SettingsService{}
}

// UpdateProfileResult holds result of profile update (errors map or nil on success).
type UpdateProfileResult struct {
	Errors map[string]string
}

// UpdateProfile validates and updates user profile. Returns errors map if validation fails.
func (s *SettingsService) UpdateProfile(userID uint, username, email, firstName, lastName string) (*UpdateProfileResult, error) {
	errors := make(map[string]string)
	if username == "" {
		errors["username"] = "Username wajib diisi"
	}
	if email == "" {
		errors["email"] = "Email wajib diisi"
	}
	var existingUser models.User
	if username != "" {
		if config.DB.Where("username = ? AND id != ?", username, userID).First(&existingUser).Error == nil {
			errors["username"] = "Username sudah digunakan"
		}
	}
	if email != "" {
		if config.DB.Where("email = ? AND id != ?", email, userID).First(&existingUser).Error == nil {
			errors["email"] = "Email sudah terdaftar"
		}
	}
	if len(errors) > 0 {
		return &UpdateProfileResult{Errors: errors}, nil
	}
	var user models.User
	if config.DB.First(&user, userID).Error != nil {
		return nil, nil
	}
	user.Username = username
	user.Email = email
	user.FirstName = firstName
	user.LastName = lastName
	if err := config.DB.Save(&user).Error; err != nil {
		errors["__all__"] = "Gagal memperbarui profil. Silakan coba lagi."
		return &UpdateProfileResult{Errors: errors}, nil
	}
	return &UpdateProfileResult{}, nil
}

// ChangePasswordResult holds result of password change (errors map or nil on success).
type ChangePasswordResult struct {
	Errors map[string]string
}

// ChangePassword validates and updates password. Returns errors map if validation fails.
func (s *SettingsService) ChangePassword(userID uint, oldPassword, newPassword1, newPassword2 string) (*ChangePasswordResult, error) {
	errors := make(map[string]string)
	if oldPassword == "" {
		errors["old_password"] = "Password lama wajib diisi"
	}
	if newPassword1 == "" {
		errors["new_password1"] = "Password baru wajib diisi"
	}
	if newPassword2 == "" {
		errors["new_password2"] = "Konfirmasi password baru wajib diisi"
	}
	if len(errors) > 0 {
		return &ChangePasswordResult{Errors: errors}, nil
	}
	if newPassword1 != newPassword2 {
		errors["new_password2"] = "Password baru tidak cocok"
		return &ChangePasswordResult{Errors: errors}, nil
	}
	if len(newPassword1) < 8 {
		errors["new_password1"] = "Password minimal 8 karakter"
		return &ChangePasswordResult{Errors: errors}, nil
	}
	var user models.User
	if config.DB.First(&user, userID).Error != nil {
		return nil, nil
	}
	if !utils.CheckPasswordHash(oldPassword, user.Password) {
		errors["old_password"] = "Password lama tidak sesuai"
		return &ChangePasswordResult{Errors: errors}, nil
	}
	hashed, err := utils.HashPassword(newPassword1)
	if err != nil {
		errors["__all__"] = "Gagal mengubah password. Silakan coba lagi."
		return &ChangePasswordResult{Errors: errors}, nil
	}
	user.Password = hashed
	if config.DB.Save(&user).Error != nil {
		errors["__all__"] = "Gagal mengubah password. Silakan coba lagi."
		return &ChangePasswordResult{Errors: errors}, nil
	}
	return &ChangePasswordResult{}, nil
}
