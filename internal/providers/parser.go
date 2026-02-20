package providers

import "strings"

type ProviderRef struct {
	Raw      string
	Name     string
	KeyAlias string
}

func ParseProviderList(raw string) []ProviderRef {
	parts := strings.Split(raw, "|")
	out := make([]ProviderRef, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ref := ProviderRef{Raw: p}
		if strings.Contains(p, ":") {
			x := strings.SplitN(p, ":", 2)
			ref.Name = strings.TrimSpace(x[0])
			ref.KeyAlias = strings.TrimSpace(x[1])
		} else {
			ref.Name = p
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		out = append(out, ProviderRef{Raw: "mock", Name: "mock"})
	}
	return out
}
