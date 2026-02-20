package util

import "testing"

func TestSanitizeTextRemovesNulAndControls(t *testing.T) {
	in := "ab\x00cd\x01\x02\n\txy"
	out := SanitizeText(in)
	if out != "abcd\n\txy" {
		t.Fatalf("unexpected sanitized output: %q", out)
	}
}
