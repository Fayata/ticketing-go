package models_test

import (
	"os"
	"reflect"
	"testing"

	"ticketing/models"
)

func TestTicketAttachmentModel_Struct(t *testing.T) {
	att := models.TicketAttachment{}

	if att.TableName() != "ticket_attachments" {
		t.Errorf("expected TableName() to be 'ticket_attachments', got '%s'", att.TableName())
	}

	attType := reflect.TypeOf(att)
	expectedFields := map[string]string{
		"ID":        "uint",
		"TicketID":  "uint",
		"ReplyID":   "*uint",
		"FileName":  "string",
		"FilePath":  "string",
		"FileSize":  "int64",
		"MimeType":  "string",
		"CreatedAt": "time.Time",
		"Ticket":    "models.Ticket",
		"Reply":     "*models.TicketReply",
	}

	for fieldName, expectedType := range expectedFields {
		f, ok := attType.FieldByName(fieldName)
		if !ok {
			t.Errorf("TicketAttachment missing field '%s'", fieldName)
			continue
		}
		if f.Type.String() != expectedType {
			t.Errorf("TicketAttachment field '%s' type = %s; want %s", fieldName, f.Type.String(), expectedType)
		}
	}
}

func TestTicketAttachment_GetFormattedSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}

	for _, tc := range tests {
		att := models.TicketAttachment{FileSize: tc.size}
		got := att.GetFormattedSize()
		if got != tc.expected {
			t.Errorf("GetFormattedSize() for %d bytes = %s; want %s", tc.size, got, tc.expected)
		}
	}
}

func TestTicket_GetInitialAttachments(t *testing.T) {
	replyID1 := uint(10)
	replyID2 := uint(11)

	ticket := models.Ticket{
		ID: 1,
		Attachments: []models.TicketAttachment{
			{ID: 1, TicketID: 1, ReplyID: nil, FileName: "init1.png"},
			{ID: 2, TicketID: 1, ReplyID: &replyID1, FileName: "reply1.png"},
			{ID: 3, TicketID: 1, ReplyID: nil, FileName: "init2.jpg"},
			{ID: 4, TicketID: 1, ReplyID: &replyID2, FileName: "reply2.png"},
		},
	}

	initial := ticket.GetInitialAttachments()
	if len(initial) != 2 {
		t.Fatalf("expected 2 initial attachments, got %d", len(initial))
	}
	if initial[0].FileName != "init1.png" || initial[1].FileName != "init2.jpg" {
		t.Errorf("unexpected initial attachments: %+v", initial)
	}
}

func TestTicketAttachment_AfterDelete(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "att_delete_test_*.png")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()

	att := models.TicketAttachment{
		FilePath: tmpPath,
	}

	if err := att.AfterDelete(nil); err != nil {
		t.Errorf("AfterDelete returned error: %v", err)
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("Expected file %s to be deleted, but it still exists", tmpPath)
	}
}

func TestTicketAttachment_IsPDF_And_IsImage(t *testing.T) {
	pdfAtt := models.TicketAttachment{
		FileName: "invoice.pdf",
		FilePath: "static/uploads/attachments/T26-0001-1725432000.pdf",
		MimeType: "application/pdf",
	}
	if !pdfAtt.IsPDF() {
		t.Errorf("expected IsPDF() to be true for PDF attachment")
	}
	if pdfAtt.IsImage() {
		t.Errorf("expected IsImage() to be false for PDF attachment")
	}

	imgAtt := models.TicketAttachment{
		FileName: "screenshot.png",
		FilePath: "static/uploads/attachments/T26-0001-1725432000.png",
		MimeType: "image/png",
	}
	if imgAtt.IsPDF() {
		t.Errorf("expected IsPDF() to be false for PNG attachment")
	}
	if !imgAtt.IsImage() {
		t.Errorf("expected IsImage() to be true for PNG attachment")
	}

	emptyAtt := models.TicketAttachment{}
	if emptyAtt.IsPDF() {
		t.Errorf("expected IsPDF() to be false for empty attachment")
	}
	if emptyAtt.IsImage() {
		t.Errorf("expected IsImage() to be false for empty attachment")
	}

	txtAtt := models.TicketAttachment{
		FileName: "notes.txt",
		FilePath: "static/uploads/attachments/notes.txt",
		MimeType: "text/plain",
	}
	if txtAtt.IsPDF() {
		t.Errorf("expected IsPDF() to be false for txt file")
	}
	if txtAtt.IsImage() {
		t.Errorf("expected IsImage() to be false for txt file")
	}
}

func TestTicketAttachment_ValueReceiverInvocation(t *testing.T) {
	ticket := models.Ticket{
		ID: 1,
		Attachments: []models.TicketAttachment{
			{
				FileName: "doc.pdf",
				FilePath: "static/uploads/attachments/T26-0001-1725432000.pdf",
				FileSize: 2048,
				MimeType: "application/pdf",
			},
			{
				FileName: "img.png",
				FilePath: "static/uploads/attachments/T26-0001-1725432000.png",
				FileSize: 512,
				MimeType: "image/png",
			},
		},
	}

	initials := ticket.GetInitialAttachments()
	if len(initials) != 2 {
		t.Fatalf("expected 2 initial attachments, got %d", len(initials))
	}

	// Calling methods on value slice items directly (as html/template does)
	if !initials[0].IsPDF() {
		t.Errorf("expected initials[0].IsPDF() to be true")
	}
	if initials[0].IsImage() {
		t.Errorf("expected initials[0].IsImage() to be false")
	}
	if initials[0].GetFormattedSize() != "2.0 KB" {
		t.Errorf("expected '2.0 KB', got %s", initials[0].GetFormattedSize())
	}

	if initials[1].IsPDF() {
		t.Errorf("expected initials[1].IsPDF() to be false")
	}
	if !initials[1].IsImage() {
		t.Errorf("expected initials[1].IsImage() to be true")
	}
	if initials[1].GetFormattedSize() != "512 B" {
		t.Errorf("expected '512 B', got %s", initials[1].GetFormattedSize())
	}
}

func TestTicketAttachment_AfterDelete_LeadingSlashAndRelative(t *testing.T) {
	// 1. Test relative path with forward slash
	tmpDir, err := os.MkdirTemp("", "att_hook_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFilePath := tmpDir + "/testfile.pdf"
	if err := os.WriteFile(testFilePath, []byte("%PDF-dummy"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	att := models.TicketAttachment{FilePath: testFilePath}
	if err := att.AfterDelete(nil); err != nil {
		t.Errorf("AfterDelete failed: %v", err)
	}
	if _, err := os.Stat(testFilePath); !os.IsNotExist(err) {
		t.Errorf("Expected %s to be deleted, but it still exists", testFilePath)
	}

	// 2. Test empty FilePath does not error
	emptyAtt := models.TicketAttachment{FilePath: ""}
	if err := emptyAtt.AfterDelete(nil); err != nil {
		t.Errorf("AfterDelete on empty FilePath returned error: %v", err)
	}
}

func TestTicket_NilPointerSafety(t *testing.T) {
	var nilTicket *models.Ticket

	if num := nilTicket.GetTicketNumber(); num != "T00-0000" {
		t.Errorf("expected 'T00-0000' for nil ticket, got %s", num)
	}
	if atts := nilTicket.GetInitialAttachments(); atts != nil {
		t.Errorf("expected nil initial attachments for nil ticket, got %+v", atts)
	}
	if status := nilTicket.GetStatusDisplay(); status != "" {
		t.Errorf("expected empty status display for nil ticket, got %s", status)
	}
	if prio := nilTicket.GetPriorityDisplay(); prio != "" {
		t.Errorf("expected empty priority display for nil ticket, got %s", prio)
	}
	if count := nilTicket.GetReplyCount(); count != 0 {
		t.Errorf("expected 0 reply count for nil ticket, got %d", count)
	}
}
