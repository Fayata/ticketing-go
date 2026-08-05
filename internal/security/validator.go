package security

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ValidateEmail checks basic email format
func ValidateEmail(email string) error {
	const emailRegex = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(emailRegex, email)
	if !matched {
		return errors.New("invalid email format")
	}
	return nil
}

// ValidateUsername checks length 3-50, alphanumeric + underscore
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 50 {
		return errors.New("username length must be between 3 and 50 characters")
	}
	for _, char := range username {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) && char != '_' {
			return errors.New("username can only contain alphanumeric characters and underscores")
		}
	}
	return nil
}

// ValidatePassword checks min 8 chars, letter, digit, special char
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	var hasLetter, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsLetter(char):
			hasLetter = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	if !hasLetter || !hasDigit || !hasSpecial {
		return errors.New("password must contain at least one letter, one digit, and one special character")
	}
	return nil
}

// ValidateTicketTitle checks length 3-200, not empty
func ValidateTicketTitle(title string) error {
	if len(title) < 3 || len(title) > 200 {
		return errors.New("ticket title must be between 3 and 200 characters")
	}
	return nil
}

// ValidateTicketDescription checks length 10-10000
func ValidateTicketDescription(desc string) error {
	if len(desc) < 10 || len(desc) > 10000 {
		return errors.New("ticket description must be between 10 and 10000 characters")
	}
	return nil
}

// SanitizeInput trims whitespace and removes null bytes
func SanitizeInput(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}

// ValidateMaxLength validates string max length
func ValidateMaxLength(field, value string, max int) error {
	if len(value) > max {
		return fmt.Errorf("%s exceeds maximum length of %d", field, max)
	}
	return nil
}
