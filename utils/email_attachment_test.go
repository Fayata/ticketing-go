package utils_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"ticketing/utils"
)

func TestBuildMIMEMessage_WithoutAttachments(t *testing.T) {
	from := "support@cloudtech.id"
	to := "user@example.com"
	subject := "Halo dari Support"
	body := "Ini adalah pesan teks pengujian."

	raw, err := utils.BuildMIMEMessage(from, to, subject, body, nil)
	if err != nil {
		t.Fatalf("BuildMIMEMessage returned unexpected error: %v", err)
	}

	msg := string(raw)
	if !strings.Contains(msg, "From: support@cloudtech.id") {
		t.Errorf("Expected From header in message, got: %s", msg)
	}
	if !strings.Contains(msg, "To: user@example.com") {
		t.Errorf("Expected To header in message, got: %s", msg)
	}
	if !strings.Contains(msg, "Subject: Halo dari Support") {
		t.Errorf("Expected Subject header in message, got: %s", msg)
	}
	if !strings.Contains(msg, "multipart/mixed") {
		t.Errorf("Expected Content-Type multipart/mixed, got: %s", msg)
	}
	if !strings.Contains(msg, body) {
		t.Errorf("Expected body in message, got: %s", msg)
	}
}

func TestBuildMIMEMessage_WithAttachments(t *testing.T) {
	from := "support@cloudtech.id"
	to := "user@example.com"
	subject := "RE: [Ticket ID: 10] Bantuan Server"
	body := "Lampiran bukti transaksi dan screenshot kendala."

	dummyPNG := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRdummy_image_data")
	dummyPDF := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")

	atts := []utils.EmailAttachment{
		{
			FileName: "screenshot.png",
			MimeType: "image/png",
			Data:     dummyPNG,
		},
		{
			FileName: "document.pdf",
			MimeType: "application/pdf",
			Data:     dummyPDF,
		},
	}

	raw, err := utils.BuildMIMEMessage(from, to, subject, body, atts)
	if err != nil {
		t.Fatalf("BuildMIMEMessage failed: %v", err)
	}

	msg := string(raw)

	// Verify headers
	if !strings.Contains(msg, "multipart/mixed") {
		t.Errorf("Expected Content-Type multipart/mixed")
	}

	// Verify attachment 1 (PNG)
	if !strings.Contains(msg, `Content-Type: image/png; name="screenshot.png"`) {
		t.Errorf("Missing PNG Content-Type header in MIME message")
	}
	if !strings.Contains(msg, `Content-Disposition: attachment; filename="screenshot.png"`) {
		t.Errorf("Missing PNG Content-Disposition header in MIME message")
	}
	encodedPNG := base64.StdEncoding.EncodeToString(dummyPNG)
	if !strings.Contains(msg, encodedPNG) {
		t.Errorf("Expected Base64 encoded PNG payload in message")
	}

	// Verify attachment 2 (PDF)
	if !strings.Contains(msg, `Content-Type: application/pdf; name="document.pdf"`) {
		t.Errorf("Missing PDF Content-Type header in MIME message")
	}
	if !strings.Contains(msg, `Content-Disposition: attachment; filename="document.pdf"`) {
		t.Errorf("Missing PDF Content-Disposition header in MIME message")
	}
	encodedPDF := base64.StdEncoding.EncodeToString(dummyPDF)
	if !strings.Contains(msg, encodedPDF) {
		t.Errorf("Expected Base64 encoded PDF payload in message")
	}
}
