package handlers

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticketing/config"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

var testPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0x63, 0x34, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

var testPDF = []byte("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\nxref\n0 2\n0000000000 65535 f \n0000000009 00000 n \ntrailer<</Size 2/Root 1 0 R>>\nstartxref\n50\n%%EOF\n")

func buildMultipartRequest(targetURL string, fields map[string]string, files map[string][]byte) (*http.Request, error) {
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

	req := httptest.NewRequest(http.MethodPost, targetURL, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func withUserContext(r *http.Request, u *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserKey, u)
	return r.WithContext(ctx)
}

func TestAttachment_CreateTicketWithAttachments(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	// Create test user
	user := models.User{
		Username:   "regularuser",
		Email:      "regular@example.com",
		IsActive:   true,
		IsVerified: true,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test company & department
	comp := models.Company{Name: "PT Test", Code: "PTT", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "Support", CompanyID: &comp.ID}
	db.Create(&dept)

	// Subtest 1: Successful creation with 2 image attachments
	{
		fields := map[string]string{
			"title":          "Issue with laptop screen",
			"description":    "Screen shows lines as seen in attachments.",
			"reply_to_email": "regular@example.com",
			"priority":       "HIGH",
			"company_id":     fmt.Sprintf("%d", comp.ID),
			"department":     fmt.Sprintf("%d", dept.ID),
		}
		files := map[string][]byte{
			"screenshot1.png": testPNG,
			"screenshot2.png": testPNG,
		}

		req, err := buildMultipartRequest("/kirim-tiket", fields, files)
		if err != nil {
			t.Fatalf("Failed to build multipart request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.CreateTicket(w, req)
		resp := w.Result()

		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Fatalf("Expected redirect status, got %d", resp.StatusCode)
		}
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected success redirect, got error in location: %s", loc)
		}

		var createdTicket models.Ticket
		if err := db.Where("created_by_id = ?", user.ID).First(&createdTicket).Error; err != nil {
			t.Fatalf("Failed to find created ticket: %v", err)
		}

		var atts []models.TicketAttachment
		if err := db.Where("ticket_id = ?", createdTicket.ID).Find(&atts).Error; err != nil {
			t.Fatalf("Failed to query attachments: %v", err)
		}

		if len(atts) != 2 {
			t.Fatalf("Expected 2 attachments in database, found %d", len(atts))
		}

		for _, a := range atts {
			if a.ReplyID != nil {
				t.Errorf("Expected initial ticket attachment to have nil ReplyID, got %v", *a.ReplyID)
			}
			if a.MimeType != "image/png" {
				t.Errorf("Expected mime_type image/png, got %s", a.MimeType)
			}
			// Clean up created file from disk
			_ = os.Remove(filepath.FromSlash(a.FilePath))
		}
	}

	// Subtest 2: Reject invalid attachment (non-image extension)
	{
		fields := map[string]string{
			"title":          "Issue with script",
			"description":    "Please see attached script.",
			"reply_to_email": "regular@example.com",
			"priority":       "LOW",
			"company_id":     fmt.Sprintf("%d", comp.ID),
			"department":     fmt.Sprintf("%d", dept.ID),
		}
		files := map[string][]byte{
			"badfile.sh": []byte("#!/bin/bash\necho hello"),
		}

		req, err := buildMultipartRequest("/kirim-tiket", fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.CreateTicket(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Fatalf("Expected error in redirect location for badfile.sh, got: %s", loc)
		}
	}

	// Subtest 3: Browser submits form with empty file input (filename="" and 0 bytes)
	{
		fields := map[string]string{
			"title":          "Ticket without image",
			"description":    "This ticket has no image attached.",
			"reply_to_email": "regular@example.com",
			"priority":       "MEDIUM",
			"company_id":     fmt.Sprintf("%d", comp.ID),
			"department":     fmt.Sprintf("%d", dept.ID),
		}
		files := map[string][]byte{
			"": {}, // Empty file input submitted by browser
		}

		req, err := buildMultipartRequest("/kirim-tiket", fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.CreateTicket(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected success for form with empty file input, got error in location: %s", loc)
		}

		var createdTicket models.Ticket
		if err := db.Where("title = ?", "Ticket without image").First(&createdTicket).Error; err != nil {
			t.Fatalf("Failed to find created ticket: %v", err)
		}

		var atts []models.TicketAttachment
		db.Where("ticket_id = ?", createdTicket.ID).Find(&atts)
		if len(atts) != 0 {
			t.Fatalf("Expected 0 attachments for ticket without images, found %d", len(atts))
		}
	}
}

func TestAttachment_UserReplyWithAttachments(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{
		Username:   "chatuser",
		Email:      "chat@example.com",
		IsActive:   true,
		IsVerified: true,
	}
	db.Create(&user)

	ticket := models.Ticket{
		Title:        "Existing Ticket",
		Description:  "Ticket description",
		CreatedByID:  user.ID,
		ReplyToEmail: user.Email,
		Status:       models.StatusWaiting,
	}
	db.Create(&ticket)

	// Subtest 1: Reply with message and image
	{
		fields := map[string]string{
			"message": "Here is additional proof",
		}
		files := map[string][]byte{
			"proof.png": testPNG,
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/tiket/%d", ticket.ID), fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.AddReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected successful reply redirect, got error: %s", loc)
		}

		var reply models.TicketReply
		if err := db.Where("ticket_id = ? AND user_id = ?", ticket.ID, user.ID).First(&reply).Error; err != nil {
			t.Fatalf("Failed to find reply: %v", err)
		}

		var replyAtts []models.TicketAttachment
		if err := db.Where("reply_id = ?", reply.ID).Find(&replyAtts).Error; err != nil {
			t.Fatalf("Failed to find reply attachment: %v", err)
		}

		if len(replyAtts) != 1 {
			t.Fatalf("Expected 1 attachment for reply, got %d", len(replyAtts))
		}
		if replyAtts[0].FileName != "proof.png" {
			t.Errorf("Expected filename proof.png, got %s", replyAtts[0].FileName)
		}
		_ = os.Remove(filepath.FromSlash(replyAtts[0].FilePath))
	}

	// Subtest 2: Reply with image only (empty message text)
	{
		fields := map[string]string{
			"message": "",
		}
		files := map[string][]byte{
			"imageonly.png": testPNG,
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/tiket/%d", ticket.ID), fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.AddReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected image-only reply to succeed, got error in location: %s", loc)
		}

		var latestReply models.TicketReply
		db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
		if latestReply.Message != "[Lampiran Gambar]" {
			t.Errorf("Expected fallback message '[Lampiran Gambar]', got %q", latestReply.Message)
		}

		var atts []models.TicketAttachment
		db.Where("reply_id = ?", latestReply.ID).Find(&atts)
		if len(atts) != 1 {
			t.Errorf("Expected 1 attachment on image-only reply, got %d", len(atts))
		}
		for _, a := range atts {
			_ = os.Remove(filepath.FromSlash(a.FilePath))
		}
	}

	// Subtest 3: Reply with empty message AND empty attachments should be rejected
	{
		fields := map[string]string{
			"message": "",
		}
		req, _ := buildMultipartRequest(fmt.Sprintf("/tiket/%d", ticket.ID), fields, nil)
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.AddReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Fatalf("Expected rejection when both message and attachments are empty, got: %s", loc)
		}
	}

	// Subtest 4: Reply with text and browser empty file input (filename="" and 0 bytes)
	{
		fields := map[string]string{
			"message": "Text reply without attachments",
		}
		files := map[string][]byte{
			"": {}, // Empty file input
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/tiket/%d", ticket.ID), fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.AddReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected text reply with empty file input to succeed, got error in location: %s", loc)
		}

		var latestReply models.TicketReply
		db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
		if latestReply.Message != "Text reply without attachments" {
			t.Errorf("Expected message %q, got %q", "Text reply without attachments", latestReply.Message)
		}
	}

	// Subtest 5: User reply to closed ticket should be rejected
	{
		closedUserTicket := models.Ticket{
			Title:        "Closed User Ticket",
			Description:  "Already solved",
			CreatedByID:  user.ID,
			ReplyToEmail: user.Email,
			Status:       models.StatusClosed,
		}
		db.Create(&closedUserTicket)

		fields := map[string]string{
			"message": "Attempt user reply on closed ticket",
		}
		files := map[string][]byte{
			"closed_proof.png": testPNG,
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/tiket/%d", closedUserTicket.ID), fields, files)
		if err != nil {
			t.Fatalf("Failed to build request: %v", err)
		}
		req = withUserContext(req, &user)
		w := httptest.NewRecorder()

		handler.AddReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Fatalf("Expected error redirect when user replies to closed ticket, got: %s", loc)
		}

		var replyCount int64
		db.Model(&models.TicketReply{}).Where("ticket_id = ?", closedUserTicket.ID).Count(&replyCount)
		if replyCount != 0 {
			t.Errorf("Expected 0 replies on closed user ticket, got %d", replyCount)
		}
	}
}

func TestAttachment_DepartmentReplyWithAttachments(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	emailService := utils.NewEmailService(cfg)
	staffService := services.NewStaffDashboardService()
	deptHandler := NewDepartmentHandler(cfg, emailService, staffService)

	dept := models.Department{Name: "Billing Dept"}
	db.Create(&dept)

	staffUser := models.User{
		Username:     "staff1",
		Email:        "staff1@example.com",
		IsActive:     true,
		IsVerified:   true,
		IsStaff:      true,
		DepartmentID: &dept.ID,
	}
	db.Create(&staffUser)

	ticket := models.Ticket{
		Title:        "Staff Ticket",
		Description:  "Needs staff help",
		DepartmentID: &dept.ID,
		AssignedToID: &staffUser.ID,
		Status:       models.StatusInProgress,
	}
	db.Create(&ticket)

	// Staff sends reply with attachment
	fields := map[string]string{
		"message": "Here is the invoice diagram",
		"status":  string(models.StatusInProgress),
	}
	files := map[string][]byte{
		"diagram.png": testPNG,
	}

	req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fields, files)
	if err != nil {
		t.Fatalf("Failed to build staff reply request: %v", err)
	}
	req = withUserContext(req, &staffUser)
	w := httptest.NewRecorder()

	deptHandler.DepartmentReply(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected successful staff reply, got error: %s", loc)
	}

	var reply models.TicketReply
	if err := db.Where("ticket_id = ? AND user_id = ?", ticket.ID, staffUser.ID).First(&reply).Error; err != nil {
		t.Fatalf("Failed to find staff reply: %v", err)
	}

	var atts []models.TicketAttachment
	db.Where("reply_id = ?", reply.ID).Find(&atts)
	if len(atts) != 1 {
		t.Fatalf("Expected 1 attachment for staff reply, got %d", len(atts))
	}
	if atts[0].FileName != "diagram.png" {
		t.Errorf("Expected attachment diagram.png, got %s", atts[0].FileName)
	}
	_ = os.Remove(filepath.FromSlash(atts[0].FilePath))

	// Subtest 2: Staff reply with text only and empty file input
	{
		fieldsTextOnly := map[string]string{
			"message": "Staff reply without attachment",
			"status":  string(models.StatusInProgress),
		}
		filesEmpty := map[string][]byte{
			"": {}, // Empty file input
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fieldsTextOnly, filesEmpty)
		if err != nil {
			t.Fatalf("Failed to build staff request: %v", err)
		}
		req = withUserContext(req, &staffUser)
		w := httptest.NewRecorder()

		deptHandler.DepartmentReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected successful staff text reply with empty file input, got: %s", loc)
		}

		var latestReply models.TicketReply
		db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
		if latestReply.Message != "Staff reply without attachment" {
			t.Errorf("Expected staff message %q, got %q", "Staff reply without attachment", latestReply.Message)
		}
	}

	// Subtest 3: Staff reply with image only (empty message text)
	{
		fieldsImageOnly := map[string]string{
			"message": "",
			"status":  string(models.StatusInProgress),
		}
		filesImageOnly := map[string][]byte{
			"staff_diagram.png": testPNG,
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fieldsImageOnly, filesImageOnly)
		if err != nil {
			t.Fatalf("Failed to build staff request: %v", err)
		}
		req = withUserContext(req, &staffUser)
		w := httptest.NewRecorder()

		deptHandler.DepartmentReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "error=") {
			t.Fatalf("Expected staff image-only reply to succeed, got error: %s", loc)
		}

		var latestReply models.TicketReply
		db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
		if latestReply.Message != "[Lampiran Gambar]" {
			t.Errorf("Expected staff fallback message '[Lampiran Gambar]', got %q", latestReply.Message)
		}

		var atts []models.TicketAttachment
		db.Where("reply_id = ?", latestReply.ID).Find(&atts)
		if len(atts) != 1 {
			t.Errorf("Expected 1 attachment for staff image-only reply, got %d", len(atts))
		}
		for _, a := range atts {
			_ = os.Remove(filepath.FromSlash(a.FilePath))
		}
	}

	// Subtest 4: Staff reply with empty message AND empty attachments should be rejected
	{
		fieldsEmpty := map[string]string{
			"message": "",
			"status":  string(models.StatusInProgress),
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fieldsEmpty, nil)
		if err != nil {
			t.Fatalf("Failed to build staff request: %v", err)
		}
		req = withUserContext(req, &staffUser)
		w := httptest.NewRecorder()

		deptHandler.DepartmentReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Fatalf("Expected error redirect for empty staff reply, got: %s", loc)
		}
	}

	// Subtest 5: Staff reply to closed ticket should be rejected
	{
		closedTicket := models.Ticket{
			Title:        "Closed Staff Ticket",
			Description:  "Already resolved",
			DepartmentID: &dept.ID,
			AssignedToID: &staffUser.ID,
			Status:       models.StatusClosed,
		}
		db.Create(&closedTicket)

		fields := map[string]string{
			"message": "Attempt reply on closed ticket",
		}
		files := map[string][]byte{
			"late_proof.png": testPNG,
		}

		req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", closedTicket.ID), fields, files)
		if err != nil {
			t.Fatalf("Failed to build staff request: %v", err)
		}
		req = withUserContext(req, &staffUser)
		w := httptest.NewRecorder()

		deptHandler.DepartmentReply(w, req)
		resp := w.Result()

		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Fatalf("Expected error redirect for replying to closed ticket, got: %s", loc)
		}

		// Verify no reply was inserted
		var replyCount int64
		db.Model(&models.TicketReply{}).Where("ticket_id = ?", closedTicket.ID).Count(&replyCount)
		if replyCount != 0 {
			t.Errorf("Expected 0 replies on closed ticket, got %d", replyCount)
		}
	}
}

func TestAttachment_SanitizePathTraversalFileName(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "pathuser", Email: "path@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)
	comp := models.Company{Name: "PT Security", Code: "PTS", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "Security Dept", CompanyID: &comp.ID}
	db.Create(&dept)

	fields := map[string]string{
		"title":          "Path traversal test",
		"description":    "Testing file name sanitization against traversal.",
		"reply_to_email": "path@example.com",
		"priority":       "LOW",
		"company_id":     fmt.Sprintf("%d", comp.ID),
		"department":     fmt.Sprintf("%d", dept.ID),
	}
	files := map[string][]byte{
		"../../../../etc/malicious.png": testPNG,
	}

	req, err := buildMultipartRequest("/kirim-tiket", fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.CreateTicket(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected ticket creation to succeed with sanitized name, got: %s", loc)
	}

	var createdTicket models.Ticket
	db.Where("title = ?", "Path traversal test").First(&createdTicket)

	var att models.TicketAttachment
	if err := db.Where("ticket_id = ?", createdTicket.ID).First(&att).Error; err != nil {
		t.Fatalf("Failed to find attachment: %v", err)
	}

	// Sanitized name should be "malicious.png" (no path traversal "../")
	if att.FileName != "malicious.png" {
		t.Errorf("Expected sanitized filename 'malicious.png', got %q", att.FileName)
	}

	// FilePath must be relative path inside static/uploads/attachments/
	if !strings.HasPrefix(att.FilePath, "static/uploads/attachments/") {
		t.Errorf("Expected FilePath to start with 'static/uploads/attachments/', got %q", att.FilePath)
	}

	_ = os.Remove(filepath.FromSlash(att.FilePath))
}

func TestAttachment_CreateTicketWithPDF_And_NamingFormat(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "pdfcreator", Email: "pdfuser@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)
	comp := models.Company{Name: "PT PDF", Code: "PTP", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "IT Support", CompanyID: &comp.ID}
	db.Create(&dept)

	fields := map[string]string{
		"title":          "Laptop crash report",
		"description":    "See the attached PDF report and screenshot.",
		"reply_to_email": "pdfuser@example.com",
		"priority":       "HIGH",
		"company_id":     fmt.Sprintf("%d", comp.ID),
		"department":     fmt.Sprintf("%d", dept.ID),
	}
	files := map[string][]byte{
		"report.pdf":  testPDF,
		"capture.png": testPNG,
	}

	req, err := buildMultipartRequest("/kirim-tiket", fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.CreateTicket(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected success redirect, got error: %s", loc)
	}

	var createdTicket models.Ticket
	if err := db.Where("created_by_id = ?", user.ID).First(&createdTicket).Error; err != nil {
		t.Fatalf("Failed to find created ticket: %v", err)
	}

	var atts []models.TicketAttachment
	db.Where("ticket_id = ?", createdTicket.ID).Find(&atts)
	if len(atts) != 2 {
		t.Fatalf("Expected 2 attachments, found %d", len(atts))
	}

	ticketNumClean := strings.TrimSpace(createdTicket.GetTicketNumber())
	ticketNumClean = strings.ReplaceAll(ticketNumClean, " ", "")

	for _, a := range atts {
		base := filepath.Base(a.FilePath)
		if !strings.HasPrefix(base, ticketNumClean+"-") {
			t.Errorf("Expected base filename to start with %q, got %q", ticketNumClean+"-", base)
		}
		if a.FileName == "report.pdf" {
			if !a.IsPDF() {
				t.Errorf("Expected report.pdf IsPDF() to be true")
			}
			if a.MimeType != "application/pdf" {
				t.Errorf("Expected mime_type application/pdf, got %s", a.MimeType)
			}
		}
		if a.FileName == "capture.png" {
			if a.IsPDF() {
				t.Errorf("Expected capture.png IsPDF() to be false")
			}
			if a.MimeType != "image/png" {
				t.Errorf("Expected mime_type image/png, got %s", a.MimeType)
			}
		}
		_ = os.Remove(filepath.FromSlash(a.FilePath))
	}
}

func TestAttachment_StaffReplyWithPDFOnly(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	emailService := utils.NewEmailService(cfg)
	staffService := services.NewStaffDashboardService()
	deptHandler := NewDepartmentHandler(cfg, emailService, staffService)

	dept := models.Department{Name: "Finance"}
	db.Create(&dept)

	staffUser := models.User{
		Username:     "staff_pdf",
		Email:        "staff_pdf@example.com",
		IsActive:     true,
		IsVerified:   true,
		IsStaff:      true,
		DepartmentID: &dept.ID,
	}
	db.Create(&staffUser)

	ticket := models.Ticket{
		Title:        "Invoice Request",
		Description:  "Please send invoice PDF",
		DepartmentID: &dept.ID,
		AssignedToID: &staffUser.ID,
		Status:       models.StatusInProgress,
	}
	db.Create(&ticket)

	fields := map[string]string{
		"message": "",
		"status":  string(models.StatusInProgress),
	}
	files := map[string][]byte{
		"invoice.pdf": testPDF,
	}

	req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &staffUser)
	w := httptest.NewRecorder()

	deptHandler.DepartmentReply(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected successful staff PDF reply, got error: %s", loc)
	}

	var latestReply models.TicketReply
	db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
	if latestReply.Message != "[Lampiran Berkas]" {
		t.Errorf("Expected staff fallback message '[Lampiran Berkas]', got %q", latestReply.Message)
	}

	var atts []models.TicketAttachment
	db.Where("reply_id = ?", latestReply.ID).Find(&atts)
	if len(atts) != 1 {
		t.Fatalf("Expected 1 attachment for staff reply, got %d", len(atts))
	}
	if atts[0].FileName != "invoice.pdf" {
		t.Errorf("Expected filename invoice.pdf, got %s", atts[0].FileName)
	}
	if !atts[0].IsPDF() {
		t.Errorf("Expected IsPDF() to be true")
	}
	_ = os.Remove(filepath.FromSlash(atts[0].FilePath))
}

func TestAttachment_UserReplyWithPDFOnly(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "user_pdf", Email: "user_pdf@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)

	ticket := models.Ticket{
		Title:        "Signed Contract",
		Description:  "Sending signed document",
		CreatedByID:  user.ID,
		ReplyToEmail: user.Email,
		Status:       models.StatusWaiting,
	}
	db.Create(&ticket)

	fields := map[string]string{
		"message": "",
	}
	files := map[string][]byte{
		"contract.pdf": testPDF,
	}

	req, err := buildMultipartRequest(fmt.Sprintf("/tiket/%d", ticket.ID), fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.AddReply(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected successful reply, got: %s", loc)
	}

	var latestReply models.TicketReply
	db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
	if latestReply.Message != "[Lampiran Berkas]" {
		t.Errorf("Expected fallback message '[Lampiran Berkas]', got %q", latestReply.Message)
	}

	var atts []models.TicketAttachment
	db.Where("reply_id = ?", latestReply.ID).Find(&atts)
	if len(atts) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(atts))
	}
	if !atts[0].IsPDF() {
		t.Errorf("Expected IsPDF() to be true")
	}
	_ = os.Remove(filepath.FromSlash(atts[0].FilePath))
}

func TestAttachment_CreateTicketWith5MixedFiles(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "mixedcreator", Email: "mixed@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)
	comp := models.Company{Name: "PT Mixed", Code: "PTM", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "IT Operations", CompanyID: &comp.ID}
	db.Create(&dept)

	fields := map[string]string{
		"title":          "Multiple issues with attachments",
		"description":    "See all 5 files attached: 3 PNG screenshots and 2 PDF reports.",
		"reply_to_email": "mixed@example.com",
		"priority":       "HIGH",
		"company_id":     fmt.Sprintf("%d", comp.ID),
		"department":     fmt.Sprintf("%d", dept.ID),
	}
	files := map[string][]byte{
		"screen1.png": testPNG,
		"report1.pdf": testPDF,
		"screen2.png": testPNG,
		"screen3.png": testPNG,
		"report2.pdf": testPDF,
	}

	req, err := buildMultipartRequest("/kirim-tiket", fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.CreateTicket(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected success redirect, got error: %s", loc)
	}

	var createdTicket models.Ticket
	if err := db.Where("created_by_id = ?", user.ID).First(&createdTicket).Error; err != nil {
		t.Fatalf("Failed to find created ticket: %v", err)
	}

	var atts []models.TicketAttachment
	db.Where("ticket_id = ?", createdTicket.ID).Order("id ASC").Find(&atts)
	if len(atts) != 5 {
		t.Fatalf("Expected 5 attachments, found %d", len(atts))
	}

	ticketNumClean := strings.TrimSpace(createdTicket.GetTicketNumber())
	ticketNumClean = strings.ReplaceAll(ticketNumClean, " ", "")

	pdfCount := 0
	imgCount := 0
	foundIndexes := make(map[string]bool)

	for _, a := range atts {
		base := filepath.Base(a.FilePath)
		if !strings.HasPrefix(base, ticketNumClean+"-") {
			t.Errorf("Expected base filename to start with %q, got %q", ticketNumClean+"-", base)
		}
		for idx := 1; idx <= 5; idx++ {
			if strings.Contains(base, fmt.Sprintf("_%d.", idx)) {
				foundIndexes[fmt.Sprintf("_%d", idx)] = true
			}
		}

		if strings.HasSuffix(a.FileName, ".pdf") {
			pdfCount++
			if !a.IsPDF() {
				t.Errorf("Expected %s IsPDF() to be true", a.FileName)
			}
			if a.IsImage() {
				t.Errorf("Expected %s IsImage() to be false", a.FileName)
			}
		} else if strings.HasSuffix(a.FileName, ".png") {
			imgCount++
			if a.IsPDF() {
				t.Errorf("Expected %s IsPDF() to be false", a.FileName)
			}
			if !a.IsImage() {
				t.Errorf("Expected %s IsImage() to be true", a.FileName)
			}
		}

		_ = os.Remove(filepath.FromSlash(a.FilePath))
	}

	if pdfCount != 2 {
		t.Errorf("Expected 2 PDF attachments, got %d", pdfCount)
	}
	if imgCount != 3 {
		t.Errorf("Expected 3 PNG attachments, got %d", imgCount)
	}
	if len(foundIndexes) != 5 {
		t.Errorf("Expected 5 unique sequence indexes (_1 through _5), got %d: %+v", len(foundIndexes), foundIndexes)
	}
}

func TestAttachment_StaffReplyWith5MixedFiles(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	emailService := utils.NewEmailService(cfg)
	staffService := services.NewStaffDashboardService()
	deptHandler := NewDepartmentHandler(cfg, emailService, staffService)

	dept := models.Department{Name: "Engineering"}
	db.Create(&dept)

	staffUser := models.User{
		Username:     "staff_mixed",
		Email:        "staff_mixed@example.com",
		IsActive:     true,
		IsVerified:   true,
		IsStaff:      true,
		DepartmentID: &dept.ID,
	}
	db.Create(&staffUser)

	ticket := models.Ticket{
		Title:        "System diagnostics",
		Description:  "Sending diagnostic logs",
		DepartmentID: &dept.ID,
		AssignedToID: &staffUser.ID,
		Status:       models.StatusInProgress,
	}
	db.Create(&ticket)

	fields := map[string]string{
		"message": "", // empty message -> should trigger fallback [Lampiran Berkas]
		"status":  string(models.StatusInProgress),
	}
	files := map[string][]byte{
		"diag1.png": testPNG,
		"spec.pdf":  testPDF,
		"diag2.png": testPNG,
		"log.pdf":   testPDF,
		"diag3.png": testPNG,
	}

	req, err := buildMultipartRequest(fmt.Sprintf("/departement/tiket/%d", ticket.ID), fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &staffUser)
	w := httptest.NewRecorder()

	deptHandler.DepartmentReply(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Expected successful staff mixed reply, got error: %s", loc)
	}

	var latestReply models.TicketReply
	db.Where("ticket_id = ?", ticket.ID).Order("id DESC").First(&latestReply)
	if latestReply.Message != "[Lampiran Berkas]" {
		t.Errorf("Expected staff fallback message '[Lampiran Berkas]', got %q", latestReply.Message)
	}

	var atts []models.TicketAttachment
	db.Where("reply_id = ?", latestReply.ID).Order("id ASC").Find(&atts)
	if len(atts) != 5 {
		t.Fatalf("Expected 5 attachments for staff reply, got %d", len(atts))
	}

	ticketNumClean := strings.TrimSpace(ticket.GetTicketNumber())
	ticketNumClean = strings.ReplaceAll(ticketNumClean, " ", "")
	foundIndexes := make(map[string]bool)

	for _, a := range atts {
		base := filepath.Base(a.FilePath)
		if !strings.HasPrefix(base, ticketNumClean+"-") {
			t.Errorf("Expected FilePath base to start with %q, got %q", ticketNumClean+"-", base)
		}
		for idx := 1; idx <= 5; idx++ {
			if strings.Contains(base, fmt.Sprintf("_%d.", idx)) {
				foundIndexes[fmt.Sprintf("_%d", idx)] = true
			}
		}
		_ = os.Remove(filepath.FromSlash(a.FilePath))
	}

	if len(foundIndexes) != 5 {
		t.Errorf("Expected 5 sequence suffixes (_1 through _5), got %d: %+v", len(foundIndexes), foundIndexes)
	}
}

func TestAttachment_RejectCorruptedPDFUpload(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "badpdfuser", Email: "badpdf@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)
	comp := models.Company{Name: "PT Bad", Code: "PTB", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "IT", CompanyID: &comp.ID}
	db.Create(&dept)

	fields := map[string]string{
		"title":          "Corrupted attachment attempt",
		"description":    "This ticket attaches a corrupted PDF file.",
		"reply_to_email": "badpdf@example.com",
		"priority":       "LOW",
		"company_id":     fmt.Sprintf("%d", comp.ID),
		"department":     fmt.Sprintf("%d", dept.ID),
	}
	files := map[string][]byte{
		"fake.pdf": []byte("This is plain text pretending to be PDF"),
	}

	req, err := buildMultipartRequest("/kirim-tiket", fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.CreateTicket(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "error=") {
		t.Fatalf("Expected rejection error redirect for fake PDF, got: %s", loc)
	}

	// Verify no ticket was left created in database
	var count int64
	db.Model(&models.Ticket{}).Where("created_by_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 tickets created after rejected upload, found %d", count)
	}
}

func TestAttachment_CreateTicket_RollbackCleansNotifications(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	_ = db.AutoMigrate(&models.TicketReply{}, &models.TicketAttachment{}, &models.Notification{})

	cfg := config.LoadConfig()
	jwtService := utils.NewJWTService(cfg)
	ticketService := services.NewTicketService(jwtService)
	emailService := utils.NewEmailService(cfg)
	handler := NewTicketHandler(cfg, emailService, ticketService)

	user := models.User{Username: "rollbackuser", Email: "rollback@example.com", IsActive: true, IsVerified: true}
	db.Create(&user)
	comp := models.Company{Name: "PT Rollback", Code: "PTR", IsActive: true}
	db.Create(&comp)
	dept := models.Department{Name: "Support", CompanyID: &comp.ID}
	db.Create(&dept)

	staff := models.User{Username: "supportstaff", Email: "staff@example.com", IsActive: true, IsVerified: true, IsStaff: true, DepartmentID: &dept.ID}
	db.Create(&staff)

	fields := map[string]string{
		"title":          "Failing ticket creation",
		"description":    "Valid description with corrupted file.",
		"reply_to_email": "rollback@example.com",
		"priority":       "LOW",
		"company_id":     fmt.Sprintf("%d", comp.ID),
		"department":     fmt.Sprintf("%d", dept.ID),
	}
	files := map[string][]byte{
		"invalid.pdf": []byte("not-a-valid-pdf"),
	}

	req, err := buildMultipartRequest("/kirim-tiket", fields, files)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req = withUserContext(req, &user)
	w := httptest.NewRecorder()

	handler.CreateTicket(w, req)
	resp := w.Result()

	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "error=") {
		t.Fatalf("Expected error redirect, got: %s", loc)
	}

	var ticketCount int64
	db.Model(&models.Ticket{}).Where("created_by_id = ?", user.ID).Count(&ticketCount)
	if ticketCount != 0 {
		t.Errorf("Expected 0 tickets after rollback, found %d", ticketCount)
	}

	var notifCount int64
	db.Model(&models.Notification{}).Where("title LIKE ?", "%Tiket baru masuk%").Count(&notifCount)
	if notifCount != 0 {
		t.Errorf("Expected 0 orphaned notifications after rollback, found %d", notifCount)
	}
}
