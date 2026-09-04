package utils

import (
	"bytes"
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
	// MaxAttachmentCount is the maximum number of files per ticket or reply.
	MaxAttachmentCount = 5

	// MaxAttachmentSizeBytes is 5 MB per file.
	MaxAttachmentSizeBytes = 5 * 1024 * 1024

	// DefaultUploadDir is the disk directory for uploaded attachments.
	DefaultUploadDir = "static/uploads/attachments"
)

// AllowedExtensions defines the permitted file extensions (lowercase with leading dot).
var AllowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".pdf":  true,
}

// AllowedMIMETypes defines permitted MIME types (images and PDF).
var AllowedMIMETypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
}

// AllowedImageMIMETypes defines permitted image MIME types.
var AllowedImageMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// SanitizeFileName cleans the original filename to prevent path traversal, injection, and XSS artifacts.
func SanitizeFileName(filename string) string {
	clean := strings.TrimSpace(filename)
	clean = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, clean)

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
	if fh.Size <= 0 {
		return "", fmt.Errorf("file \"%s\" kosong (0 byte)", fh.Filename)
	}
	if fh.Size > MaxAttachmentSizeBytes {
		sizeMB := float64(fh.Size) / (1024.0 * 1024.0)
		return "", fmt.Errorf("ukuran file \"%s\" (%.1f MB) melebihi batas maksimal 5 MB", fh.Filename, sizeMB)
	}

	// 2. Check file extension
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !AllowedExtensions[ext] {
		return "", fmt.Errorf("format file \"%s\" tidak didukung. Hanya gambar (JPG, JPEG, PNG, GIF, WebP) dan dokumen PDF yang diperbolehkan", fh.Filename)
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

	// 4. Validate content type & magic bytes based on extension
	if ext == ".pdf" {
		if !bytes.HasPrefix(buf, []byte("%PDF-")) {
			return "", fmt.Errorf("tipe konten file \"%s\" tidak valid. Dokumen PDF harus memiliki header %%PDF-", fh.Filename)
		}
		detected := http.DetectContentType(buf)
		if detected != "application/pdf" && !strings.HasPrefix(detected, "application/pdf") {
			return "", fmt.Errorf("tipe konten file \"%s\" (%s) tidak valid. Dokumen PDF tidak valid", fh.Filename, detected)
		}
		return "application/pdf", nil
	}

	detected := http.DetectContentType(buf)

	// WebP check: RIFF....WEBP in the first 12 bytes
	if len(buf) >= 12 && string(buf[0:4]) == "RIFF" && string(buf[8:12]) == "WEBP" {
		detected = "image/webp"
	}

	if !AllowedImageMIMETypes[detected] {
		return "", fmt.Errorf("tipe konten file \"%s\" (%s) tidak valid. Hanya file gambar asli yang diperbolehkan", fh.Filename, detected)
	}

	return detected, nil
}

// GenerateTicketAttachmentFileName creates filename in format [NomorTiket]-[Timestamp].[ext]
// or with sequence index suffix [NomorTiket]-[Timestamp]_[index].[ext] when multiple files are uploaded.
func GenerateTicketAttachmentFileName(ticketNumber string, timestamp int64, seqIndex int, totalFiles int, originalName string) string {
	cleanTicket := strings.TrimSpace(ticketNumber)
	cleanTicket = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == ' ' || r < 32 || r == 127 {
			return -1
		}
		return r
	}, cleanTicket)
	cleanTicket = strings.ReplaceAll(cleanTicket, "..", "")
	if cleanTicket == "" {
		cleanTicket = "T00-0000"
	}
	if timestamp <= 0 {
		timestamp = time.Now().Unix()
	}
	if seqIndex < 1 {
		seqIndex = 1
	}
	ext := strings.ToLower(filepath.Ext(originalName))
	if totalFiles > 1 {
		return fmt.Sprintf("%s-%d_%d%s", cleanTicket, timestamp, seqIndex, ext)
	}
	return fmt.Sprintf("%s-%d%s", cleanTicket, timestamp, ext)
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

// SaveUploadedAttachmentWithTicket validates and saves a single uploaded file header with ticket-based filename.
func SaveUploadedAttachmentWithTicket(fh *multipart.FileHeader, uploadDir string, ticketNumber string, timestamp int64, seqIndex int, totalFiles int) (*models.TicketAttachment, error) {
	mimeType, err := ValidateAttachmentHeader(fh)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("gagal membuat direktori upload: %w", err)
	}

	cleanOrigName := SanitizeFileName(fh.Filename)
	var uniqueName string
	if strings.TrimSpace(ticketNumber) != "" {
		uniqueName = GenerateTicketAttachmentFileName(ticketNumber, timestamp, seqIndex, totalFiles, cleanOrigName)
	} else {
		var genErr error
		uniqueName, genErr = GenerateUniqueFileName(cleanOrigName)
		if genErr != nil {
			return nil, fmt.Errorf("gagal menghasilkan nama file unik: %w", genErr)
		}
	}

	// Atomically create file with collision avoidance using O_EXCL
	ext := filepath.Ext(uniqueName)
	base := strings.TrimSuffix(uniqueName, ext)
	counter := 0
	var destFile *os.File
	var destPath string

	for {
		candidateName := uniqueName
		if counter > 0 {
			candidateName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		}
		candidatePath := filepath.Join(uploadDir, candidateName)

		f, openErr := os.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if openErr == nil {
			destFile = f
			destPath = candidatePath
			uniqueName = candidateName
			break
		}
		if os.IsExist(openErr) || errors.Is(openErr, os.ErrExist) {
			counter++
			if counter > 10000 {
				return nil, fmt.Errorf("gagal membuat file tujuan unik setelah %d percobaan: %w", counter, openErr)
			}
			continue
		}
		return nil, fmt.Errorf("gagal membuat file tujuan: %w", openErr)
	}

	srcFile, err := fh.Open()
	if err != nil {
		_ = destFile.Close()
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("gagal membuka file sumber: %w", err)
	}
	defer srcFile.Close()

	// Limit reading to MaxAttachmentSizeBytes + 1 to detect and prevent oversized streams
	written, err := io.Copy(destFile, io.LimitReader(srcFile, MaxAttachmentSizeBytes+1))
	_ = destFile.Close()
	if err != nil && err != io.EOF {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("gagal menyimpan file: %w", err)
	}
	if written > MaxAttachmentSizeBytes {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("ukuran file \"%s\" melebihi batas maksimal 5 MB", fh.Filename)
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

// SaveUploadedAttachment validates and saves a single uploaded file header to disk.
func SaveUploadedAttachment(fh *multipart.FileHeader, uploadDir string) (*models.TicketAttachment, error) {
	return SaveUploadedAttachmentWithTicket(fh, uploadDir, "", 0, 0, 0)
}

// ValidateMultipartRequest checks if files in multipart request are valid without saving to disk.
func ValidateMultipartRequest(r *http.Request, formKey string) error {
	if r.MultipartForm == nil {
		ct := r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			if strings.HasPrefix(strings.ToLower(ct), "multipart/") {
				return fmt.Errorf("gagal memproses data formulir multipart: %w", err)
			}
			return nil
		}
	}

	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil
	}

	rawFiles := r.MultipartForm.File[formKey]
	if len(rawFiles) == 0 {
		return nil
	}

	var files []*multipart.FileHeader
	for _, fh := range rawFiles {
		if fh != nil && strings.TrimSpace(fh.Filename) != "" {
			files = append(files, fh)
		}
	}

	if len(files) == 0 {
		return nil
	}

	if len(files) > MaxAttachmentCount {
		return fmt.Errorf("maksimal %d file yang dapat dilampirkan (ditemukan %d)", MaxAttachmentCount, len(files))
	}

	for _, fh := range files {
		if _, err := ValidateAttachmentHeader(fh); err != nil {
			return err
		}
	}

	return nil
}

// ProcessMultipartAttachments parses the request multipart form, validates all attachments,
// and saves them to disk. If ticketNumbers is provided, filenames follow [NomorTiket]-[Timestamp].[ext].
func ProcessMultipartAttachments(r *http.Request, formKey string, uploadDir string, ticketNumbers ...string) ([]models.TicketAttachment, error) {
	if uploadDir == "" {
		uploadDir = DefaultUploadDir
	}

	// Parse up to 32 MB multipart memory
	if r.MultipartForm == nil {
		ct := r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			if strings.HasPrefix(strings.ToLower(ct), "multipart/") {
				return nil, fmt.Errorf("gagal memproses lampiran formulir multipart: %w", err)
			}
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
		return nil, fmt.Errorf("maksimal %d file yang dapat dilampirkan (ditemukan %d)", MaxAttachmentCount, len(files))
	}

	// Pre-validate all files before writing any file to disk
	for _, fh := range files {
		if _, err := ValidateAttachmentHeader(fh); err != nil {
			return nil, err
		}
	}

	ticketNumber := ""
	if len(ticketNumbers) > 0 {
		ticketNumber = strings.TrimSpace(ticketNumbers[0])
	}
	timestamp := time.Now().Unix()

	var results []models.TicketAttachment
	var savedPaths []string

	for i, fh := range files {
		att, err := SaveUploadedAttachmentWithTicket(fh, uploadDir, ticketNumber, timestamp, i+1, len(files))
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
			p := filepath.FromSlash(a.FilePath)
			if err := os.Remove(p); err != nil {
				trimmed := strings.TrimLeft(a.FilePath, "/\\")
				if trimmed != "" && trimmed != a.FilePath {
					_ = os.Remove(filepath.FromSlash(trimmed))
				}
			}
		}
	}
}
