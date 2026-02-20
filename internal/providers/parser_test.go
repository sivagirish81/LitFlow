package providers

import "testing"

func TestParseProviderList(t *testing.T) {
	refs := ParseProviderList("mock|openai:key1|openai:key2")
	if len(refs) != 3 {
		t.Fatalf("expected 3 providers got %d", len(refs))
	}
	if refs[1].Name != "openai" || refs[1].KeyAlias != "key1" {
		t.Fatalf("unexpected parse result: %+v", refs[1])
	}
}
