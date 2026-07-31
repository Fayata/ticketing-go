package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"ticketing/config"
	"ticketing/models"
)

type AIService struct {
	client *genai.Client
}

type AIFilters struct {
	Department string `json:"department"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	Keyword    string `json:"keyword"`
}

func NewAIService(cfg *config.Config) *AIService {
	if cfg.GoogleAPIKey == "" {
		log.Println("[Warning] Google API Key is not set in environment.")
		return &AIService{}
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GoogleAPIKey))
	if err != nil {
		log.Printf("Failed to create Gemini client: %v", err)
		return &AIService{}
	}

	return &AIService{
		client: client,
	}
}

// TranslateQueryToFilters converts natural language into JSON filters using Gemini.
func (s *AIService) TranslateQueryToFilters(ctx context.Context, naturalQuery string) (AIFilters, error) {
	if s.client == nil {
		return AIFilters{Keyword: naturalQuery}, nil
	}

	model := s.client.GenerativeModel("gemini-3.5-flash")
	model.ResponseMIMEType = "application/json"
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(`Kamu adalah AI Query Translator untuk sistem ticketing helpdesk.
Tugas: Ubah pertanyaan bahasa Indonesia dari admin menjadi parameter filter JSON.

ATURAN PENTING:
- Jika user hanya minta "tampilkan/tampilin/lihat semua tiket/user/departemen" TANPA filter spesifik, kembalikan SEMUA field kosong (artinya tampilkan semua data).
- Kata "user", "tiket", "departemen", "tampilkan", "tampilin", "lihat", "cari" adalah KATA PERINTAH, BUKAN keyword pencarian. Jangan masukkan kata-kata ini ke field keyword.
- Field "keyword" HANYA untuk istilah teknis spesifik yang dicari di judul/deskripsi tiket (misal: "error 500", "printer rusak", "login gagal").
- Jika tidak ada keyword teknis spesifik, kosongkan field keyword.

Schema JSON:
{
  "department": "nama departemen jika disebut (IT, HR, Finance, Technical Support, Customer Service), kosong jika tidak",
  "status": "WAITING atau IN_PROGRESS atau CLOSED, kosong jika tidak disebut",
  "priority": "LOW atau MEDIUM atau HIGH atau URGENT, kosong jika tidak disebut",
  "keyword": "istilah teknis spesifik saja, kosong jika tidak ada"
}

Contoh:
- "tampilin semua tiket" → {"department":"","status":"","priority":"","keyword":""}
- "tampilin user" → {"department":"","status":"","priority":"","keyword":""}
- "tampilin departemen" → {"department":"","status":"","priority":"","keyword":""}
- "tiket di departemen IT yang masih open" → {"department":"IT","status":"WAITING","priority":"","keyword":""}
- "cari tiket tentang printer rusak" → {"department":"","status":"","priority":"","keyword":"printer rusak"}
- "tiket urgent yang belum selesai" → {"department":"","status":"WAITING","priority":"URGENT","keyword":""}
- "berapa tiket yang dikirim user" → {"department":"","status":"","priority":"","keyword":""}`),
		},
	}

	prompt := genai.Text(naturalQuery)
	resp, err := model.GenerateContent(ctx, prompt)
	if err != nil {
		log.Printf("Gemini generation error: %v", err)
		return AIFilters{Keyword: naturalQuery}, err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return AIFilters{Keyword: naturalQuery}, fmt.Errorf("empty response from Gemini")
	}

	part := resp.Candidates[0].Content.Parts[0]
	textResponse, ok := part.(genai.Text)
	if !ok {
		return AIFilters{Keyword: naturalQuery}, fmt.Errorf("unexpected response type from Gemini")
	}

	jsonStr := strings.TrimSpace(string(textResponse))
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var filters AIFilters
	err = json.Unmarshal([]byte(jsonStr), &filters)
	if err != nil {
		log.Printf("Failed to unmarshal Gemini JSON: %v. JSON was: %s", err, jsonStr)
		return AIFilters{Keyword: naturalQuery}, err
	}

	return filters, nil
}

// AnalyzeTickets sends ticket data + user question to Gemini and returns an AI summary/answer.
func (s *AIService) AnalyzeTickets(ctx context.Context, question string, tickets []models.Ticket) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("AI client not available")
	}

	// Build a concise summary of ticket data for AI context
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Total tiket ditemukan: %d\n\n", len(tickets)))
	for i, t := range tickets {
		dept := "-"
		if t.Department != nil {
			dept = t.Department.Name
		}
		creator := "-"
		if t.CreatedBy.Username != "" {
			name := strings.TrimSpace(t.CreatedBy.FirstName + " " + t.CreatedBy.LastName)
			if name == "" {
				name = t.CreatedBy.Username
			}
			creator = name
		}
		sb.WriteString(fmt.Sprintf("%d. #%d | %s | Dept: %s | Status: %s | Priority: %s | Pemohon: %s | Tanggal: %s\n",
			i+1, t.ID, t.Title, dept, t.Status, t.Priority, creator, t.CreatedAt.Format("02 Jan 2006")))
	}

	model := s.client.GenerativeModel("gemini-3.5-flash")
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(`Kamu adalah asisten analitik untuk sistem ticketing. Jawab pertanyaan admin berdasarkan data tiket yang diberikan.
Berikan jawaban yang ringkas, akurat, dan langsung menjawab pertanyaan.
Gunakan bahasa Indonesia. Jika pertanyaan berupa hitungan, berikan angkanya.
Jika pertanyaan berupa analisis, berikan insight yang berguna.
Format jawaban dengan rapi menggunakan bullet point atau angka jika diperlukan.
Jangan menambahkan informasi yang tidak ada di data.`),
		},
	}

	prompt := genai.Text(fmt.Sprintf("Pertanyaan admin: %s\n\nData tiket:\n%s", question, sb.String()))
	resp, err := model.GenerateContent(ctx, prompt)
	if err != nil {
		log.Printf("Gemini analyze error: %v", err)
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	part := resp.Candidates[0].Content.Parts[0]
	textResponse, ok := part.(genai.Text)
	if !ok {
		return "", fmt.Errorf("unexpected response type")
	}

	return strings.TrimSpace(string(textResponse)), nil
}
