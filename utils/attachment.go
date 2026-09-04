package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ticketing/models"
)

const (
	// MaxAttachmentCount is the maximum number of images per ticket or reply.
	MaxAttachmentCount = 5

	// MaxAttachmentSizeBytes is 5 MB per file.
	MaxAttachmentSizeBytes = 5 * 1024 * 1024

	// DefaultUploadDir is the disk directory for uploaded attachments.
	DefaultUploadDir = "static/uploads/attachments"
)

// AllowedExtensions defines the permitted image extensions (lowercase with leading dot).
var AllowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// AllowedMIMETypes defines permitted image MIME types.
var AllowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// SanitizeFileName cleans the original filename to prevent path traversal, injection, and XSS artifacts.
func SanitizeFileName(filename string) string {
	clean := strings.TrimSpace(filename)
	clean = strings.ReplaceAll(clean, "\x00", "")
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")

	// Handle HTML tags before path separation so </script> doesn't split on /
	if strings.Contains(clean, "<") || strings.Contains(clean, ">") {
		clean = strings.ReplaceAll(clean, "</", "")
		clean = strings.ReplaceAll(clean, "<", "")
		clean = strings.ReplaceAll(clean, ">", "")
	}

	// Normalize slashes
	clean = strings.ReplaceAll(clean, "\\", "/")
	// Extract base name from path
	if idx := strings.LastIndex(clean, "/"); idx >= 0 {
		clean = clean[idx+1:]
	}

	clean = strings.ReplaceAll(clean, "\"", "")
	clean = strings.ReplaceAll(clean, "'", "")
	clean = strings.ReplaceAll(clean, ";", "")

	// Extract extension before stripping dots
	ext := filepath.Ext(clean)
	// If ext is only dots (like "." or ".."), it's not a valid extension
	if strings.Trim(ext, ".") == "" {
		ext = ""
	}

	// Base without extension
	baseWithoutExt := clean
	if ext != "" {
		baseWithoutExt = strings.TrimSuffix(clean, ext)
	}

	// Strip remaining traversal dots and spaces from base
	baseWithoutExt = strings.ReplaceAll(baseWithoutExt, "..", "")
	baseWithoutExt = strings.Trim(baseWithoutExt, " .")

	if baseWithoutExt == "" {
		if ext != "" {
			return "attachment" + ext
		}
		return "attachment"
	}

	return baseWithoutExt + ext
}

// ValidateAttachmentHeader checks file size, extension, and content type magic bytes.
func ValidateAttachmentHeader(fh *multipart.FileHeader) (mimeType string, err error) {
	if fh == nil {
		return "", errors.New("file lampiran kosong")
	}

	// 1. Check file size
	if fh.Size == 0 {
		return "", fmt.Errorf("file \"%s\" kosong (0 byte)", fh.Filename)
	}
	if fh.Size > MaxAttachmentSizeBytes {
		sizeMB := float64(fh.Size) / (1024.0 * 1024.0)
		return "", fmt.Errorf("ukuran file \"%s\" (%.1f MB) melebihi batas maksimal 5 MB", fh.Filename, sizeMB)
	}

	// 2. Check file extension
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !AllowedExtensions[ext] {
		return "", fmt.Errorf("format file \"%s\" tidak didukung. Hanya gambar (JPG, JPEG, PNG, GIF, WebP) yang diperbolehkan", fh.Filename)
	}

	// 3. Open file to inspect magic bytes
	file, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("gagal membaca file \"%s\": %w", fh.Filename, err)
	}
	defer file.Close()

	// Read initial 512 bytes for MIME detection
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("gagal memeriksa konten file \"%s\": %w", fh.Filename, err)
	}
	buf = buf[:n]

	detected := http.DetectContentType(buf)

	// WebP check: RIFF....WEBP in the first 12 bytes
	if len(buf) >= 12 && string(buf[0:4]) == "RIFF" && string(buf[8:12]) == "WEBP" {
		detected = "image/webp"
	}

	if !AllowedMIMETypes[detected] {
		return "", fmt.Errorf("tipe konten file \"%s\" (%s) tidak valid. Hanya file gambar asli yang diperbolehkan", fh.Filename, detected)
	}

	return detected, nil
}

// GenerateUniqueFileName creates a collision-resistant, non-guessable filename.
func GenerateUniqueFileName(originalName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	randHex := hex.EncodeToString(randomBytes)
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%d_%s%s", timestamp, randHex, ext), nil
}

// SaveUploadedAttachment validates and saves a single uploaded file header to disk.
func SaveUploadedAttachment(fh *multipart.FileHeader, uploadDir string) (*models.TicketAttachment, error) {
	mimeType, err := ValidateAttachmentHeader(fh)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("gagal membuat direktori upload: %w", err)
	}

	cleanOrigName := SanitizeFileName(fh.Filename)
	uniqueName, err := GenerateUniqueFileName(cleanOrigName)
	if err != nil {
		return nil, fmt.Errorf("gagal menghasilkan nama file unik: %w", err)
	}

	destPath := filepath.Join(uploadDir, uniqueName)
	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat file tujuan: %w", err)
	}

	srcFile, err := fh.Open()
	if err != nil {
		_ = destFile.Close()
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("gagal membuka file sumber: %w", err)
	}
	defer srcFile.Close()

	written, err := io.Copy(destFile, srcFile)
	_ = destFile.Close()
	if err != nil {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("gagal menyimpan file: %w", err)
	}

	// Store path normalized with forward slashes for URLs
	webPath := filepath.ToSlash(filepath.Join(uploadDir, uniqueName))

	attachment := &models.TicketAttachment{
		FileName: cleanOrigName,
		FilePath: webPath,
		FileSize: written,
		MimeType: mimeType,
	}

	return attachment, nil
}

// ProcessMultipartAttachments parses the request multipart form, validates all attachments,
// and saves them to disk. If any file fails validation or saving, all saved files are cleaned up.
// Empty file inputs submitted when no file is chosen are ignored.
func ProcessMultipartAttachments(r *http.Request, formKey string, uploadDir string) ([]models.TicketAttachment, error) {
	if uploadDir == "" {
		uploadDir = DefaultUploadDir
	}

	// Parse up to 32 MB multipart memory
	if r.MultipartForm == nil {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			// If not multipart form or error parsing, return nil without error if no files expected
			return nil, nil
		}
	}

	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil, nil
	}

	rawFiles := r.MultipartForm.File[formKey]
	if len(rawFiles) == 0 {
		return nil, nil
	}

	// Filter out empty file inputs submitted by browsers when user does not select any file
	var files []*multipart.FileHeader
	for _, fh := range rawFiles {
		if fh != nil && strings.TrimSpace(fh.Filename) != "" {
			files = append(files, fh)
		}
	}

	if len(files) == 0 {
		return nil, nil
	}

	if len(files) > MaxAttachmentCount {
		return nil, fmt.Errorf("maksimal %d file gambar yang dapat dilampirkan (ditemukan %d)", MaxAttachmentCount, len(files))
	}

	// Pre-validate all files before writing any file to disk
	for _, fh := range files {
		if _, err := ValidateAttachmentHeader(fh); err != nil {
			return nil, err
		}
	}

	var results []models.TicketAttachment
	var savedPaths []string

	for _, fh := range files {
		att, err := SaveUploadedAttachment(fh, uploadDir)
		if err != nil {
			// Clean up any files saved so far
			for _, p := range savedPaths {
				_ = os.Remove(filepath.FromSlash(p))
			}
			return nil, err
		}
		savedPaths = append(savedPaths, att.FilePath)
		results = append(results, *att)
	}

	return results, nil
}

// CleanupAttachments removes the given attachments from disk.
func CleanupAttachments(attachments []models.TicketAttachment) {
	for _, a := range attachments {
		if a.FilePath != "" {
			_ = os.Remove(filepath.FromSlash(a.FilePath))
		}
	}
}
