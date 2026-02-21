package providers

import "testing"

func TestResolveOllamaEmbedModel_Default(t *testing.T) {
	t.Setenv("LITFLOW_OLLAMA_EMBED_MODEL", "")
	got := resolveOllamaEmbedModel("")
	if got != "nomic-embed-text" {
		t.Fatalf("expected default nomic-embed-text, got %q", got)
	}
}

func TestMatchDimension(t *testing.T) {
	src := []float32{1, 2, 3}
	a := matchDimension(src, 2)
	if len(a) != 2 || a[0] != 1 || a[1] != 2 {
		t.Fatalf("truncate failed: %#v", a)
	}
	b := matchDimension(src, 5)
	if len(b) != 5 || b[0] != 1 || b[2] != 3 || b[3] != 0 || b[4] != 0 {
		t.Fatalf("pad failed: %#v", b)
	}
}
