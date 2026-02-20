package util

import "testing"

func TestChunkText(t *testing.T) {
	text := "abcdefghijklmnopqrstuvwxyz"
	chunks := ChunkText(text, 10, 2)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != "abcdefghij" {
		t.Fatalf("unexpected first chunk: %s", chunks[0])
	}
}
