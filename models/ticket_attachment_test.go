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
