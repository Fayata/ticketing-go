package main

import (
	"log"
	"strings"

	"ticketing/config"
	"ticketing/models"
)

func main() {
	log.Println("Memulai pembersihan tiket spam/SQL injection...")
	
	cfg := config.LoadConfig()
	if err := config.InitDatabase(cfg); err != nil {
		log.Fatal("Gagal koneksi ke database: ", err)
	}

	var tickets []models.Ticket
	// Ambil semua tiket (termasuk yang mungkin sudah di soft-delete)
	config.DB.Unscoped().Find(&tickets)

	spamCount := 0
	for _, t := range tickets {
		title := strings.ToUpper(t.Title)
		
		// Deteksi pattern SQL Injection dari gambar
		isSpam := strings.Contains(title, "UNION SELECT") || 
		          strings.Contains(title, "1=1") || 
		          strings.Contains(title, "'OR '1'='1") ||
		          strings.Contains(title, "' OR '1'='1") ||
		          strings.Contains(title, "VERSION()")

		if isSpam {
			log.Printf("Menemukan tiket spam: [%s] %s", t.GetTicketNumber(), t.Title)
			
			// Hard delete relasi terkait terlebih dahulu (Unscoped = hapus permanen)
			config.DB.Unscoped().Where("ticket_id = ?", t.ID).Delete(&models.TicketReply{})
			config.DB.Unscoped().Where("ticket_id = ?", t.ID).Delete(&models.TicketAssignmentHistory{})
			config.DB.Unscoped().Where("ticket_id = ?", t.ID).Delete(&models.TicketRating{})
			
			// Hard delete tiket utama
			config.DB.Unscoped().Delete(&t)
			
			spamCount++
		}
	}

	log.Printf("Pembersihan selesai! Berhasil menghapus secara permanen %d tiket spam beserta data relasinya.", spamCount)
}
