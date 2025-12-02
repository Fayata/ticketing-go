package utils

import (
	"fmt"
	"log"
	"net/smtp"

	"ticketing/config"
)

type EmailService struct {
	cfg *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// SendMail mengirim email menggunakan style sederhana sesuai request
func (e *EmailService) SendMail(to, subject, body string) error {
	// Konfigurasi Auth
	auth := smtp.PlainAuth("", e.cfg.EmailUsername, e.cfg.EmailPassword, e.cfg.EmailHost)

	// Format alamat server (host:port)
	addr := fmt.Sprintf("%s:%d", e.cfg.EmailHost, e.cfg.EmailPort)

	// Header dan Body Email
	// Kita gunakan text/plain agar format baris baru (\n) di body tetap terbaca rapi
	// Jika ingin HTML, ganti Content-Type jadi text/html dan ganti \n di body jadi <br>
	msg := []byte("From: " + e.cfg.EmailFrom + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-version: 1.0;\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\";\r\n\r\n" +
		body + "\r\n")

	// Kirim Email
	// Disini 'to' diambil dinamis dari parameter fungsi, bukan hardcoded
	err := smtp.SendMail(addr, auth, e.cfg.EmailFrom, []string{to}, msg)

	if err != nil {
		log.Printf("❌ Error sending email to %s: %v", to, err)
		return err
	}

	log.Printf("✅ Email sent to %s successfully", to)
	return nil
}

// SendTicketConfirmation mengirim email konfirmasi pembuatan tiket
func (e *EmailService) SendTicketConfirmation(to, username, title string, ticketID uint, department, priority, status, description string) error {
	subject := fmt.Sprintf("[Ticket ID: %d] %s", ticketID, title)

	body := fmt.Sprintf(`Halo %s,

Terima kasih telah menghubungi kami. Tiket Anda telah berhasil dibuat dengan rincian berikut:

ID Tiket  : %d
Judul     : %s
Departemen: %s
Prioritas : %s
Status    : %s

Deskripsi:
%s

---
Tim support kami akan segera meninjau tiket Anda.
Mohon menunggu balasan dari tim support melalui email ini.

Salam,
Tim Support`, username, ticketID, title, department, priority, status, description)

	return e.SendMail(to, subject, body)
}

// SendTicketReply mengirim notifikasi balasan tiket
func (e *EmailService) SendTicketReply(to, username, title string, ticketID uint, status, replyMessage, replierName string) error {
	subject := fmt.Sprintf("RE: [Ticket ID: %d] %s", ticketID, title)

	body := fmt.Sprintf(`Halo %s,

Tim support kami (%s) telah membalas tiket Anda:

---
%s
---

Detail Tiket:
ID Tiket    : %d
Judul       : %s
Status      : %s

Silakan balas email ini jika ada pertanyaan tambahan.

Salam,
%s
Tim Support`, username, replierName, replyMessage, ticketID, title, status, replierName)

	return e.SendMail(to, subject, body)
}
