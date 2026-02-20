package util

import "strings"

// SanitizeText removes bytes and control characters that Postgres text columns reject
// (especially NUL / 0x00 from some PDF extractors).
func SanitizeText(s string) string {
	if s == "" {
		return s
	}
	// NUL bytes are not valid in PostgreSQL text.
	s = strings.ReplaceAll(s, "\x00", "")

	// Drop other non-printing controls except common whitespace.
	r := make([]rune, 0, len(s))
	for _, ch := range s {
		if ch == '\n' || ch == '\r' || ch == '\t' {
			r = append(r, ch)
			continue
		}
		if ch < 0x20 {
			continue
		}
		r = append(r, ch)
	}
	return strings.TrimSpace(string(r))
}
