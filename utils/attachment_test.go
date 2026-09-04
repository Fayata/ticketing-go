package utils_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ticketing/models"
	"ticketing/utils"
)

// Minimal valid image payloads
var (
	pngBytes = []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0x63, 0x34, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}

	gifBytes = []byte(
		"GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;",
	)

	jpegBytes = []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xff, 0xdb, 0x00, 0x43,
		0x00, 0x03, 0x02, 0x02, 0x03, 0x02, 0x02, 0x03, 0x03, 0x03, 0x03, 0x04,
		0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11,
		0x00, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00, 0xbf,
		0x00, 0xff, 0xd9,
	}

	webpBytes = []byte{
		'R', 'I', 'F', 'F', 0x1a, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P',
		'V', 'P', '8', ' ', 0x0e, 0x00, 0x00, 0x00, 0x30, 0x01, 0x00, 0x9d,
		0x01, 0x2a, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00, 0x34, 0x25,
	}

	jpegExifBytes = []byte{
		0xff, 0xd8, 0xff, 0xe1, 0x00, 0x16, 'E', 'x', 'i', 'f', 0x00, 0x00,
		'I', 'I', 0x2a, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xdb, 0x00, 0x43, 0x00, 0x03, 0x02, 0x02, 0x03, 0x02, 0x02, 0x03,
		0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11,
		0x00, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00, 0xbf,
		0x00, 0xff, 0xd9,
	}

	webpVP8XBytes = []byte{
		'R', 'I', 'F', 'F', 0x20, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X', 0x0a, 0x00, 0x00, 0x00, 0x12, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	animatedGifBytes = []byte(
		"GIF89a\x02\x00\x02\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x00\x0a\x00\x00\x00,\x00\x00\x00\x00\x02\x00\x02\x00\x00\x02\x02D\x01\x00!\xf9\x04\x00\x0a\x00\x00\x00,\x00\x00\x00\x00\x02\x00\x02\x00\x00\x02\x02D\x01\x00;",
	)
)

func createMultipartRequest(t *testing.T, formKey string, files map[string][]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for filename, content := range files {
		part, err := writer.CreateFormFile(formKey, filename)
		if err != nil {
			t.Fatalf("CreateFormFile failed: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("Write part failed: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/test-upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestSanitizeFileName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"photo.png", "photo.png"},
		{"../../etc/passwd", "passwd"},
		{"..\\..\\windows\\system32\\cmd.exe", "cmd.exe"},
		{"my image file (1).jpg", "my image file (1).jpg"},
		{"evil\x00file.png", "evilfile.png"},
		{"line\r\nbreak.png", "linebreak.png"},
		{"<script>alert(1)</script>.png", "scriptalert(1)script.png"},
		{"\"quoted\".png", "quoted.png"},
		{"file;drop.jpg", "filedrop.jpg"},
		{".png", "attachment.png"},
		{"..png", "attachment.png"},
		{"", "attachment"},
		{"...", "attachment"},
		{"///", "attachment"},
	}

	for _, tc := range cases {
		got := utils.SanitizeFileName(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeFileName(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestGenerateUniqueFileName(t *testing.T) {
	name1, err1 := utils.GenerateUniqueFileName("test.png")
	if err1 != nil {
		t.Fatalf("GenerateUniqueFileName failed: %v", err1)
	}
	name2, err2 := utils.GenerateUniqueFileName("test.png")
	if err2 != nil {
		t.Fatalf("GenerateUniqueFileName failed: %v", err2)
	}

	if name1 == name2 {
		t.Errorf("expected unique names, but got identical: %s", name1)
	}
	if !strings.HasSuffix(name1, ".png") {
		t.Errorf("expected .png extension, got %s", name1)
	}
}

func TestValidateAttachmentHeader_ValidImages(t *testing.T) {
	cases := []struct {
		filename string
		content  []byte
		wantMIME string
	}{
		{"photo.png", pngBytes, "image/png"},
		{"picture.jpg", jpegBytes, "image/jpeg"},
		{"graphic.jpeg", jpegBytes, "image/jpeg"},
		{"animation.gif", gifBytes, "image/gif"},
		{"banner.webp", webpBytes, "image/webp"},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			req := createMultipartRequest(t, "attachments", map[string][]byte{tc.filename: tc.content})
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("ParseMultipartForm failed: %v", err)
			}
			fh := req.MultipartForm.File["attachments"][0]

			mimeType, err := utils.ValidateAttachmentHeader(fh)
			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if mimeType != tc.wantMIME {
				t.Errorf("ValidateAttachmentHeader() = %s; want %s", mimeType, tc.wantMIME)
			}
		})
	}
}

func TestValidateAttachmentHeader_RejectsExceedingSize(t *testing.T) {
	// Create payload slightly exceeding 5 MB
	largePayload := make([]byte, utils.MaxAttachmentSizeBytes+1)
	copy(largePayload, pngBytes) // Starts with PNG header but too large

	req := createMultipartRequest(t, "attachments", map[string][]byte{"large.png": largePayload})
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("ParseMultipartForm failed: %v", err)
	}
	fh := req.MultipartForm.File["attachments"][0]

	_, err := utils.ValidateAttachmentHeader(fh)
	if err == nil {
		t.Fatal("expected size validation error for >5MB file, got nil")
	}
	if !strings.Contains(err.Error(), "melebihi batas maksimal 5 MB") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateAttachmentHeader_RejectsInvalidExtension(t *testing.T) {
	invalidExtensions := []string{
		"script.sh", "program.exe", "document.pdf", "image.svg", "file.php", "data.json",
	}

	for _, filename := range invalidExtensions {
		t.Run(filename, func(t *testing.T) {
			req := createMultipartRequest(t, "attachments", map[string][]byte{filename: pngBytes})
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("ParseMultipartForm failed: %v", err)
			}
			fh := req.MultipartForm.File["attachments"][0]

			_, err := utils.ValidateAttachmentHeader(fh)
			if err == nil {
				t.Fatalf("expected extension error for %s, got nil", filename)
			}
			if !strings.Contains(err.Error(), "tidak didukung") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}

func TestValidateAttachmentHeader_RejectsFakeImage(t *testing.T) {
	// File with valid image extension (.png), but containing HTML/script/text
	fakePNG := []byte("<html><script>alert('xss')</script></html>")

	req := createMultipartRequest(t, "attachments", map[string][]byte{"fake.png": fakePNG})
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("ParseMultipartForm failed: %v", err)
	}
	fh := req.MultipartForm.File["attachments"][0]

	_, err := utils.ValidateAttachmentHeader(fh)
	if err == nil {
		t.Fatal("expected MIME detection error for fake image, got nil")
	}
	if !strings.Contains(err.Error(), "tidak valid") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProcessMultipartAttachments_MaxFiles(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "ticketing_test_uploads_max")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Test 5 files (allowed)
	files5 := map[string][]byte{
		"f1.png": pngBytes,
		"f2.png": pngBytes,
		"f3.jpg": jpegBytes,
		"f4.gif": gifBytes,
		"f5.png": pngBytes,
	}
	req5 := createMultipartRequest(t, "attachments", files5)
	atts, err := utils.ProcessMultipartAttachments(req5, "attachments", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error for 5 files: %v", err)
	}
	if len(atts) != 5 {
		t.Fatalf("expected 5 attachments saved, got %d", len(atts))
	}

	// Test 6 files (rejected)
	files6 := map[string][]byte{
		"f1.png": pngBytes,
		"f2.png": pngBytes,
		"f3.jpg": jpegBytes,
		"f4.gif": gifBytes,
		"f5.png": pngBytes,
		"f6.webp": webpBytes,
	}
	req6 := createMultipartRequest(t, "attachments", files6)
	_, err6 := utils.ProcessMultipartAttachments(req6, "attachments", tmpDir)
	if err6 == nil {
		t.Fatal("expected error when attaching 6 files, got nil")
	}
	if !strings.Contains(err6.Error(), "maksimal 5 file") {
		t.Errorf("unexpected error message: %v", err6)
	}
}

func TestProcessMultipartAttachments_RollbackOnFailure(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "ticketing_test_rollback")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Mix valid and invalid files
	mixedFiles := map[string][]byte{
		"good.png": pngBytes,
		"bad.txt":  []byte("plain text content"),
	}
	req := createMultipartRequest(t, "attachments", mixedFiles)
	_, err := utils.ProcessMultipartAttachments(req, "attachments", tmpDir)
	if err == nil {
		t.Fatal("expected error due to bad.txt, got nil")
	}

	// Verify no orphaned files remain in tmpDir
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) > 0 {
		t.Errorf("expected 0 leftover files in directory after rollback, found %d", len(entries))
	}
}

func TestValidateAttachmentHeader_ZeroByteFile(t *testing.T) {
	req := createMultipartRequest(t, "attachments", map[string][]byte{"empty.png": {}})
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("ParseMultipartForm failed: %v", err)
	}
	fh := req.MultipartForm.File["attachments"][0]

	_, err := utils.ValidateAttachmentHeader(fh)
	if err == nil {
		t.Fatal("expected error for 0-byte file, got nil")
	}
	if !strings.Contains(err.Error(), "kosong (0 byte)") {
		t.Errorf("unexpected error message for 0-byte file: %v", err)
	}
}

func TestValidateAttachmentHeader_EXIFAndAnimated(t *testing.T) {
	cases := []struct {
		filename string
		content  []byte
		wantMIME string
	}{
		{"photo_exif.jpg", jpegExifBytes, "image/jpeg"},
		{"banner_vp8x.webp", webpVP8XBytes, "image/webp"},
		{"animated.gif", animatedGifBytes, "image/gif"},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			req := createMultipartRequest(t, "attachments", map[string][]byte{tc.filename: tc.content})
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("ParseMultipartForm failed: %v", err)
			}
			fh := req.MultipartForm.File["attachments"][0]

			mimeType, err := utils.ValidateAttachmentHeader(fh)
			if err != nil {
				t.Fatalf("unexpected validation error for %s: %v", tc.filename, err)
			}
			if mimeType != tc.wantMIME {
				t.Errorf("ValidateAttachmentHeader(%s) = %s; want %s", tc.filename, mimeType, tc.wantMIME)
			}
		})
	}
}

func TestProcessMultipartAttachments_BrowserEmptyFileInput(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "ticketing_test_empty_input")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Simulate standard browser submitting form with empty file input: filename="" and 0 bytes
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("attachments", "")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_ = part // write nothing

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/test-upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	atts, err := utils.ProcessMultipartAttachments(req, "attachments", tmpDir)
	if err != nil {
		t.Fatalf("expected no error when browser submits empty file input, got: %v", err)
	}
	if len(atts) != 0 {
		t.Fatalf("expected 0 attachments for empty file input, got %d", len(atts))
	}
}

func TestProcessMultipartAttachments_Exact5MBBoundary(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "ticketing_test_5mb_boundary")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Create payload of exactly 5 MB
	exact5MBPayload := make([]byte, utils.MaxAttachmentSizeBytes)
	copy(exact5MBPayload, pngBytes)

	req := createMultipartRequest(t, "attachments", map[string][]byte{"exact5mb.png": exact5MBPayload})
	atts, err := utils.ProcessMultipartAttachments(req, "attachments", tmpDir)
	if err != nil {
		t.Fatalf("expected exact 5MB file to be accepted, got error: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment saved, got %d", len(atts))
	}
	if atts[0].FileSize != int64(utils.MaxAttachmentSizeBytes) {
		t.Errorf("expected file size %d, got %d", utils.MaxAttachmentSizeBytes, atts[0].FileSize)
	}
}

func TestCleanupAttachments(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "ticketing_test_cleanup")
	_ = os.RemoveAll(tmpDir)
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	filePath1 := filepath.Join(tmpDir, "file1.png")
	filePath2 := filepath.Join(tmpDir, "file2.png")
	_ = os.WriteFile(filePath1, pngBytes, 0644)
	_ = os.WriteFile(filePath2, pngBytes, 0644)

	atts := []models.TicketAttachment{
		{FilePath: filepath.ToSlash(filePath1)},
		{FilePath: filepath.ToSlash(filePath2)},
	}

	utils.CleanupAttachments(atts)

	if _, err := os.Stat(filePath1); !os.IsNotExist(err) {
		t.Errorf("expected file1.png to be deleted, but still exists")
	}
	if _, err := os.Stat(filePath2); !os.IsNotExist(err) {
		t.Errorf("expected file2.png to be deleted, but still exists")
	}
}

func TestConcurrentUniqueFileNames(t *testing.T) {
	const count = 50
	names := make([]string, count)
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			name, err := utils.GenerateUniqueFileName("photo.png")
			if err != nil {
				t.Errorf("GenerateUniqueFileName failed: %v", err)
				return
			}
			names[idx] = name
		}(i)
	}

	wg.Wait()

	seen := make(map[string]bool)
	for _, n := range names {
		if n == "" {
			t.Errorf("empty filename generated")
		}
		if seen[n] {
			t.Errorf("duplicate filename generated under concurrency: %s", n)
		}
		seen[n] = true
	}
}
