package util

import (
	"sort"
	"strings"
	"unicode"
)

func DisplaySnippet(s string, maxRunes int) string {
	return trimClean(s, maxRunes)
}

// DisplayEvidenceSnippet extracts the most relevant sentence(s) for a query.
func DisplayEvidenceSnippet(chunkText, query string, maxRunes int) string {
	chunkText = trimClean(chunkText, 4000)
	if chunkText == "" {
		return ""
	}
	queryTerms := meaningfulTerms(query)
	if len(queryTerms) == 0 {
		return trimClean(chunkText, maxRunes)
	}

	sentences := splitSentences(chunkText)
	if len(sentences) == 0 {
		return trimClean(chunkText, maxRunes)
	}

	type scored struct {
		sentence string
		score    int
	}
	list := make([]scored, 0, len(sentences))
	for _, s := range sentences {
		low := strings.ToLower(s)
		score := 0
		for _, term := range queryTerms {
			if strings.Contains(low, term) {
				score++
			}
		}
		list = append(list, scored{sentence: s, score: score})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score == list[j].score {
			return len(list[i].sentence) < len(list[j].sentence)
		}
		return list[i].score > list[j].score
	})

	best := strings.TrimSpace(list[0].sentence)
	if best == "" {
		return trimClean(chunkText, maxRunes)
	}
	if len(list) > 1 && list[1].score > 0 {
		combo := best + " " + strings.TrimSpace(list[1].sentence)
		return trimClean(combo, maxRunes)
	}
	return trimClean(best, maxRunes)
}

func splitSentences(s string) []string {
	out := make([]string, 0, 8)
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			x := strings.TrimSpace(b.String())
			if x != "" {
				out = append(out, x)
			}
			b.Reset()
		}
	}
	rest := strings.TrimSpace(b.String())
	if rest != "" {
		out = append(out, rest)
	}
	return out
}

func meaningfulTerms(s string) []string {
	s = strings.ToLower(trimClean(s, 2000))
	fields := strings.Fields(s)
	stop := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {},
		"for": {}, "is": {}, "are": {}, "was": {}, "were": {}, "what": {}, "how": {}, "why": {},
		"which": {}, "that": {}, "this": {}, "these": {}, "those": {}, "with": {}, "from": {}, "across": {},
	}
	uniq := map[string]struct{}{}
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ",.;:!?()[]{}\"'`")
		if len(f) < 3 {
			continue
		}
		if _, ok := stop[f]; ok {
			continue
		}
		if _, ok := uniq[f]; ok {
			continue
		}
		uniq[f] = struct{}{}
		terms = append(terms, f)
	}
	return terms
}

func trimClean(s string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 420
	}
	s = SanitizeText(s)
	s = restoreWordBoundaries(s)
	s = normalizeWhitespace(s)

	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !unicode.IsPrint(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) || unicode.IsPunct(r) {
			out = append(out, r)
			continue
		}
	}
	trimmed := strings.TrimSpace(string(out))
	runes := []rune(trimmed)
	if len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes])) + "..."
	}
	return trimmed
}

func restoreWordBoundaries(s string) string {
	if s == "" {
		return s
	}
	in := []rune(s)
	out := make([]rune, 0, len(in)+len(in)/8)
	for i, r := range in {
		if i > 0 {
			prev := in[i-1]
			if needBoundary(prev, r) {
				last := out[len(out)-1]
				if !unicode.IsSpace(last) {
					out = append(out, ' ')
				}
			}
		}
		out = append(out, r)
	}
	return string(out)
}

func needBoundary(a, b rune) bool {
	if unicode.IsLower(a) && unicode.IsUpper(b) {
		return true
	}
	if unicode.IsLetter(a) && unicode.IsDigit(b) {
		return true
	}
	if unicode.IsDigit(a) && unicode.IsLetter(b) {
		return true
	}
	return false
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
