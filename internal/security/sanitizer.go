package security

import (
	"bytes"
	"html"
	"html/template"
	"strings"
)

// SanitizeHTML escapes all HTML entities
func SanitizeHTML(s string) string {
	return html.EscapeString(s)
}

// SafeLinebreaks escapes HTML first, THEN converts newlines to <br>
func SafeLinebreaks(s string) template.HTML {
	escaped := html.EscapeString(s)
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return template.HTML(escaped)
}

// SanitizeForJS escapes </script> tags in JSON data for safe embedding
func SanitizeForJS(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("</script>"), []byte("<\\/script>"))
}
