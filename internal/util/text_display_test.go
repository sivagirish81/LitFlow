package util

import (
	"strings"
	"testing"
)

func TestDisplaySnippet(t *testing.T) {
	in := "Hello\x00   world \n\t C\\u0001"
	out := DisplaySnippet(in, 100)
	if out == "" {
		t.Fatalf("expected non-empty snippet")
	}
}

func TestDisplayEvidenceSnippet(t *testing.T) {
	chunk := "This paper studies edge computing in cloud schedulers. It evaluates latency reduction for edge workloads. Unrelated appendix text."
	q := "What are edge workload latency results?"
	out := DisplayEvidenceSnippet(chunk, q, 200)
	if !strings.Contains(strings.ToLower(out), "latency") {
		t.Fatalf("expected relevance to latency in snippet, got: %q", out)
	}
}
