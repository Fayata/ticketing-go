import re

with open('handlers/admin.go', 'r', encoding='utf-8') as f:
    content = f.read()

# Replace AdminHandler struct
content = re.sub(
    r'type AdminHandler struct \{\s*cfg\s*\*config\.Config\s*adminDashService\s*\*services\.AdminDashboardService\s*\}',
    'type AdminHandler struct {\n\tcfg              *config.Config\n\tadminDashService *services.AdminDashboardService\n\taiService        *services.AIService\n\tadminSearch      *services.AdminSearchService\n}',
    content
)

# Replace NewAdminHandler
content = re.sub(
    r'func NewAdminHandler\(cfg \*config\.Config, adminDashService \*services\.AdminDashboardService\) \*AdminHandler \{\s*return &AdminHandler\{cfg: cfg, adminDashService: adminDashService\}\s*\}',
    'func NewAdminHandler(cfg *config.Config, adminDashService *services.AdminDashboardService, aiService *services.AIService, adminSearch *services.AdminSearchService) *AdminHandler {\n\treturn &AdminHandler{cfg: cfg, adminDashService: adminDashService, aiService: aiService, adminSearch: adminSearch}\n}',
    content
)

# Add import for utils if not exists
if '"ticketing/utils"' not in content:
    content = content.replace('"ticketing/services"', '"ticketing/services"\n\t"ticketing/utils"')

search_code = """

// SearchAdmin handles the natural language search via Google AI
func (h *AdminHandler) SearchAdmin(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, config.Path("admin/dashboard"), http.StatusSeeOther)
		return
	}

	filters, err := h.aiService.TranslateQueryToFilters(r.Context(), query)
	if err != nil {
		log.Printf("Error translating query via AI: %v", err)
		// Fallback to basic keyword search if AI fails
		filters = services.AIFilters{Keyword: query}
	}

	tickets, err := h.adminSearch.SearchTickets(filters)
	if err != nil {
		log.Printf("Error searching tickets: %v", err)
	}

	utils.RenderTemplate(w, r, "admin_search_results", map[string]interface{}{
		"title":   "Hasil Pencarian: " + query,
		"query":   query,
		"filters": filters,
		"tickets": tickets,
	})
}
"""
content += search_code

with open('handlers/admin.go', 'w', encoding='utf-8') as f:
    f.write(content)
