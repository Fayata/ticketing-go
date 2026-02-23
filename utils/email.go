package utils

import (
	"crypto/tls"
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

// SendMail dengan support dual mode (465 SSL & 587 STARTTLS)
func (e *EmailService) SendMail(to, subject, body string) error {
	// 1. Setup Headers
	headers := make(map[string]string)
	headers["From"] = e.cfg.EmailFrom
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=\"UTF-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	addr := fmt.Sprintf("%s:%d", e.cfg.EmailHost, e.cfg.EmailPort)
	host := e.cfg.EmailHost

	var client *smtp.Client
	var err error

	// KONFIGURASI TLS: InsecureSkipVerify true agar tidak rewel sertifikat
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// LOGIKA UTAMA: Pilih metode koneksi berdasarkan Port
	if e.cfg.EmailPort == 465 {
		// --- METODE PORT 465 (SMTPS / Implicit SSL) ---
		// Langsung connect pakai TLS, bypass STARTTLS handshake yang sering error
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			log.Printf("❌ Gagal connect SSL (Port 465): %v", err)
			return err
		}

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			log.Printf("❌ Gagal membuat client SMTP: %v", err)
			return err
		}
		defer client.Close()

		log.Println("✅ Terhubung via SMTPS (Port 465)")

	} else {
		// --- METODE PORT 587 (STARTTLS) ---
		// Fallback untuk port 587/25
		client, err = smtp.Dial(addr)
		if err != nil {
			log.Printf("❌ Gagal dial (Port %d): %v", e.cfg.EmailPort, err)
			return err
		}
		defer client.Close()

		// Coba STARTTLS jika didukung
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(tlsConfig); err != nil {
				log.Printf("❌ Gagal STARTTLS: %v", err)
				return err
			}
		}
	}

	// 2. Authenticate
	// Menggunakan PlainAuth. Jika server butuh LOGIN auth, bisa ditambahkan nanti.
	auth := smtp.PlainAuth("", e.cfg.EmailUsername, e.cfg.EmailPassword, host)
	if err = client.Auth(auth); err != nil {
		log.Printf("❌ Gagal Auth: %v", err)
		return err
	}

	// 3. Kirim Email
	if err = client.Mail(e.cfg.EmailFrom); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(message))
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	if err = client.Quit(); err != nil {
		// Error saat quit bisa diabaikan kadang-kadang
		log.Printf("⚠️ Note: Quit error (biasanya aman): %v", err)
	}

	log.Printf("✅ Email SUKSES terkirim ke %s", to)
	return nil
}

// Helper functions wrapper (tidak berubah)
func (e *EmailService) SendTicketConfirmation(to, username, title string, ticketID uint, department, priority, status, description string) error {
	subject := fmt.Sprintf("[Ticket ID: %d] %s", ticketID, title)
	body := fmt.Sprintf("Halo %s,\n\nTiket #%d berhasil dibuat.\nJudul: %s\n\nDeskripsi:\n%s\n\nSalam,\nTim Support", username, ticketID, title, description)
	return e.SendMail(to, subject, body)
}

func (e *EmailService) SendTicketReply(to, username, title string, ticketID uint, status, replyMessage, replierName string) error {
	subject := fmt.Sprintf("RE: [Ticket ID: %d] %s", ticketID, title)
	body := fmt.Sprintf("Halo %s,\n\nAda balasan baru dari %s:\n\n%s\n\nSalam,\nTim Support", username, replierName, replyMessage)
	return e.SendMail(to, subject, body)
}

func (e *EmailService) SendRatingRequest(to, username, title string, ticketID uint, ratingToken string) error {
	subject := fmt.Sprintf("Rating Pengalaman - Tiket #%d", ticketID)
	ratingURL := fmt.Sprintf("%s/rating/%d?token=%s", e.cfg.BaseURL, ticketID, ratingToken)
	body := fmt.Sprintf(`Halo %s,

Terima kasih telah menggunakan layanan support kami!

Tiket Anda #%d dengan judul "%s" telah ditutup.

Kami sangat menghargai feedback Anda. Mohon luangkan waktu sejenak untuk memberikan rating pengalaman Anda:

%s

Rating Anda sangat membantu kami untuk meningkatkan kualitas layanan.

Terima kasih,
Tim Support`, username, ticketID, title, ratingURL)
	return e.SendMail(to, subject, body)
}