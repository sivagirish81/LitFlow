package util

import "strings"

func ChunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 1200
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 0
	}
	runes := []rune(text)
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}
	out := make([]string, 0)
	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		part := strings.TrimSpace(string(runes[i:end]))
		if part != "" {
			out = append(out, part)
		}
		if end == len(runes) {
			break
		}
	}
	return out
}
