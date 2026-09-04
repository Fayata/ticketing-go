package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ticketing/config"
)

var dummyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0x63, 0x34, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func buildMultipart(fields map[string]string, files map[string][]byte) (*http.Request, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}

	for filename, content := range files {
		part, err := writer.CreateFormFile("attachments", filename)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(content); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/kirim-tiket", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func TestValidateTicketInput_ValidMultipart(t *testing.T) {
	config.AppBasePath = "/Ticketing"

	fields := map[string]string{
		"title":          "Laptop monitor broken",
		"description":    "Screen flickers whenever the laptop is turned on.",
		"reply_to_email": "user@example.com",
		"priority":       "HIGH",
	}
	files := map[string][]byte{
		"photo1.png": dummyPNG,
		"photo2.png": dummyPNG,
	}

	req, err := buildMultipart(fields, files)
	if err != nil {
		t.Fatalf("buildMultipart error: %v", err)
	}

	var passed bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passed = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	ValidateTicketInput(next).ServeHTTP(rec, req)

	if !passed {
		t.Fatalf("Expected ValidateTicketInput to pass, but it redirected with status %d, location: %s",
			rec.Code, rec.Header().Get("Location"))
	}
}

func TestValidateTicketInput_TooManyAttachments(t *testing.T) {
	config.AppBasePath = "/Ticketing"

	fields := map[string]string{
		"title":          "Valid title here",
		"description":    "Valid description longer than 10 characters.",
		"reply_to_email": "user@example.com",
		"priority":       "MEDIUM",
	}
	files := make(map[string][]byte)
	for i := 1; i <= 6; i++ {
		files[fmt.Sprintf("img%d.png", i)] = dummyPNG
	}

	req, err := buildMultipart(fields, files)
	if err != nil {
		t.Fatalf("buildMultipart error: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Handler should not have been called for >5 attachments")
	})

	rec := httptest.NewRecorder()
	ValidateTicketInput(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/Ticketing/kirim-tiket?error=") {
		t.Errorf("Expected redirect to start with '/Ticketing/kirim-tiket?error=', got: %s", loc)
	}
	if !strings.Contains(loc, url.QueryEscape("Maksimal 5 file gambar yang dapat dilampirkan")) {
		t.Errorf("Expected error message for >5 attachments, got: %s", loc)
	}
}

func TestValidateTicketInput_InvalidFields(t *testing.T) {
	config.AppBasePath = "/Ticketing"

	// Short title
	{
		fields := map[string]string{
			"title":          "ab",
			"description":    "Valid description longer than 10 chars.",
			"reply_to_email": "user@example.com",
			"priority":       "LOW",
		}
		req, _ := buildMultipart(fields, nil)
		rec := httptest.NewRecorder()
		ValidateTicketInput(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, url.QueryEscape("Judul tiket harus antara 3 hingga 200 karakter")) {
			t.Errorf("Expected short title error, got: %s", loc)
		}
	}

	// Invalid email
	{
		fields := map[string]string{
			"title":          "Valid Title Here",
			"description":    "Valid description longer than 10 chars.",
			"reply_to_email": "invalid-email",
			"priority":       "LOW",
		}
		req, _ := buildMultipart(fields, nil)
		rec := httptest.NewRecorder()
		ValidateTicketInput(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, url.QueryEscape("Format alamat email tidak valid")) {
			t.Errorf("Expected invalid email error, got: %s", loc)
		}
	}
}

func TestInputSanitizer_Multipart(t *testing.T) {
	fields := map[string]string{
		"title":       "  Title with null\x00 and spaces  ",
		"description": "  Description text  ",
	}

	req, _ := buildMultipart(fields, nil)

	var sanitizedTitle, sanitizedDesc string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sanitizedTitle = r.FormValue("title")
		sanitizedDesc = r.FormValue("description")
	})

	rec := httptest.NewRecorder()
	InputSanitizer(next).ServeHTTP(rec, req)

	if sanitizedTitle != "Title with null and spaces" {
		t.Errorf("Expected sanitized title 'Title with null and spaces', got %q", sanitizedTitle)
	}
	if sanitizedDesc != "Description text" {
		t.Errorf("Expected sanitized description 'Description text', got %q", sanitizedDesc)
	}
}
