package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ticketing/config"
)

type EmailService struct {
	cfg *config.Config
}

// EmailAttachment menyimpan metadata dan path/data berkas lampiran untuk dikirim via email.
type EmailAttachment struct {
	FileName string
	FilePath string
	MimeType string
	Data     []byte
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// sendRawMail mengirimkan raw MIME message melalui koneksi SMTP (Port 465 SSL atau 587 STARTTLS).
func (e *EmailService) sendRawMail(to string, rawMessage []byte) error {
	addr := fmt.Sprintf("%s:%d", e.cfg.EmailHost, e.cfg.EmailPort)
	host := e.cfg.EmailHost

	var client *smtp.Client
	var err error

	skipVerify := false
	if e.cfg != nil {
		skipVerify = e.cfg.EmailInsecureSkipVerify
	}

	log.Printf("[Email] Connecting to %s (Port %d, SkipVerify=%v)", addr, e.cfg.EmailPort, skipVerify)

	// [Security] TLS config — proper certificate verification with optional skip verify for shared hosting
	tlsConfig := &tls.Config{
		InsecureSkipVerify: skipVerify,
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
	}

	// LOGIKA UTAMA: Pilih metode koneksi berdasarkan Port
	if e.cfg.EmailPort == 465 {
		// --- METODE PORT 465 (SMTPS / Implicit SSL) ---
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

	// Authenticate
	auth := smtp.PlainAuth("", e.cfg.EmailUsername, e.cfg.EmailPassword, host)
	if err = client.Auth(auth); err != nil {
		log.Printf("❌ Gagal Auth: %v", err)
		return err
	}

	// Kirim Email
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
	_, err = w.Write(rawMessage)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	if err = client.Quit(); err != nil {
		log.Printf("⚠️ Note: Quit error (biasanya aman): %v", err)
	}

	log.Printf("✅ Email SUKSES terkirim ke %s", to)
	return nil
}

// SendMail dengan support dual mode (465 SSL & 587 STARTTLS) untuk email teks biasa.
func (e *EmailService) SendMail(to, subject, body string) error {
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

	return e.sendRawMail(to, []byte(message))
}

// BuildMIMEMessage mengonstruksi raw payload email MIME multipart/mixed dengan berkas lampiran fisik.
func BuildMIMEMessage(from, to, subject, body string, attachments []EmailAttachment) ([]byte, error) {
	boundary := fmt.Sprintf("----=_NextPart_%d_%d", time.Now().UnixNano(), os.Getpid())
	var buf bytes.Buffer

	// Headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	buf.WriteString("\r\n")

	// Part 1: Body teks pesan
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n\r\n")

	// Part 2..N: Lampiran Berkas
	for _, att := range attachments {
		fileData := att.Data
		if len(fileData) == 0 && att.FilePath != "" {
			data, err := os.ReadFile(att.FilePath)
			if err != nil {
				log.Printf("⚠️ Gagal membaca berkas lampiran email %s: %v", att.FilePath, err)
				continue
			}
			fileData = data
		}
		if len(fileData) == 0 {
			continue
		}

		fileName := att.FileName
		if fileName == "" {
			fileName = filepath.Base(att.FilePath)
		}
		if fileName == "" || fileName == "." {
			fileName = "attachment"
		}

		mimeType := att.MimeType
		if mimeType == "" {
			ext := strings.ToLower(filepath.Ext(fileName))
			switch ext {
			case ".pdf":
				mimeType = "application/pdf"
			case ".png":
				mimeType = "image/png"
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".gif":
				mimeType = "image/gif"
			case ".webp":
				mimeType = "image/webp"
			default:
				mimeType = http.DetectContentType(fileData)
			}
		}

		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", mimeType, fileName))
		buf.WriteString("Content-Transfer-Encoding: base64\r\n")
		buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", fileName))
		buf.WriteString("\r\n")

		// Encode Base64 dengan batas 76 karakter per baris (RFC 2045)
		encoded := base64.StdEncoding.EncodeToString(fileData)
		for len(encoded) > 76 {
			buf.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}
		buf.WriteString(encoded + "\r\n")
	}

	buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	return buf.Bytes(), nil
}

// SendMailWithAttachments mengirim email dengan berkas lampiran fisik (MIME multipart/mixed).
func (e *EmailService) SendMailWithAttachments(to, subject, body string, attachments []EmailAttachment) error {
	if len(attachments) == 0 {
		return e.SendMail(to, subject, body)
	}

	rawMsg, err := BuildMIMEMessage(e.cfg.EmailFrom, to, subject, body, attachments)
	if err != nil {
		return err
	}

	return e.sendRawMail(to, rawMsg)
}

// Helper functions wrapper
func (e *EmailService) SendTicketConfirmationWithAttachments(to, username, title string, ticketID uint, department, priority, status, description string, attachments []EmailAttachment) error {
	year := time.Now().Format("06")
	ticketNum := fmt.Sprintf("T%s-%04d", year, ticketID)
	subject := fmt.Sprintf("[%s] %s", ticketNum, title)
	body := fmt.Sprintf("Halo %s,\n\nKami telah menerima request anda dan tiket %s sudah di assign ke tim terkait.\nJudul: %s\n\nDeskripsi:\n%s\n\nTerima kasih atas kesediannya untuk menunggu serta kerjasamanya", username, ticketNum, title, description)
	if len(attachments) > 0 {
		body += fmt.Sprintf("\n\n(Terdapat %d berkas lampiran yang disertakan)", len(attachments))
	}
	body += "\n\nSalam,\nTim Support"
	return e.SendMailWithAttachments(to, subject, body, attachments)
}

func (e *EmailService) SendTicketConfirmation(to, username, title string, ticketID uint, department, priority, status, description string) error {
	return e.SendTicketConfirmationWithAttachments(to, username, title, ticketID, department, priority, status, description, nil)
}

func (e *EmailService) SendTicketReplyWithAttachments(to, username, title string, ticketID uint, status, replyMessage, replierName string, attachments []EmailAttachment) error {
	year := time.Now().Format("06")
	ticketNum := fmt.Sprintf("T%s-%04d", year, ticketID)
	subject := fmt.Sprintf("RE: [%s] %s", ticketNum, title)
	body := fmt.Sprintf("Halo %s,\n\nAda balasan baru dari %s pada tiket %s:\n\n%s", username, replierName, ticketNum, replyMessage)
	if len(attachments) > 0 {
		body += fmt.Sprintf("\n\n(Terdapat %d berkas lampiran yang disertakan)", len(attachments))
	}
	body += "\n\nSalam,\nTim Support"
	return e.SendMailWithAttachments(to, subject, body, attachments)
}

func (e *EmailService) SendTicketReply(to, username, title string, ticketID uint, status, replyMessage, replierName string) error {
	return e.SendTicketReplyWithAttachments(to, username, title, ticketID, status, replyMessage, replierName, nil)
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