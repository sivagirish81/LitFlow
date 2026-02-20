package providers

import "testing"

func TestResolveGroqKeyFallback(t *testing.T) {
	_ = t
	// Key resolution is environment-dependent; this test ensures constructor does not panic.
	p := NewGroqProvider("alias1")
	if p == nil {
		t.Fatalf("expected provider instance")
	}
}
