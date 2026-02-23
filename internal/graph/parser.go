package graph

import (
	"encoding/json"
	"strings"
)

func ParseTriplesJSON(raw string) []Triple {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = stripCodeFence(raw)
	var payload struct {
		Triples []Triple `json:"triples"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	out := make([]Triple, 0, len(payload.Triples))
	seen := map[string]struct{}{}
	for _, t := range payload.Triples {
		n, ok := NormalizeTriple(t)
		if !ok {
			continue
		}
		k := string(n.SourceType) + "|" + n.SourceName + "|" + string(n.RelationType) + "|" + string(n.TargetType) + "|" + n.TargetName
		if _, exists := seen[k]; exists {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, n)
	}
	return out
}

func stripCodeFence(s string) string {
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
