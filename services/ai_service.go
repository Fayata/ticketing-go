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
		// Fallback if no API key: just use the query as keyword
		return AIFilters{Keyword: naturalQuery}, nil
	}

	model := s.client.GenerativeModel("gemini-3.5-flash")
	model.ResponseMIMEType = "application/json"
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(`You are an AI Query Translator for a ticketing system database.
Extract filter parameters from the user's natural language query..
Return ONLY a valid JSON object matching this schema:
{
  "department": "string (extract department name if mentioned, e.g., 'IT', 'HR', 'Finance', else empty string)",
  "status": "string (map to one of: 'OPEN', 'IN_PROGRESS', 'RESOLVED', 'CLOSED', else empty string)",
  "priority": "string (map to one of: 'LOW', 'MEDIUM', 'HIGH', 'URGENT', else empty string)",
  "keyword": "string (any remaining important keywords or error messages, else empty string)"
}
Example: "tampilkan tiket yang statusnya open di departemen IT yang error"
JSON: {"department": "IT", "status": "OPEN", "priority": "", "keyword": "error"}`),
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
