package graph

import (
	"regexp"
	"strings"
)

var ws = regexp.MustCompile(`\s+`)

func CanonicalName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = ws.ReplaceAllString(s, " ")
	return s
}

func NormalizeTriple(t Triple) (Triple, bool) {
	t.SourceType = EntityType(strings.ToLower(strings.TrimSpace(string(t.SourceType))))
	t.TargetType = EntityType(strings.ToLower(strings.TrimSpace(string(t.TargetType))))
	t.RelationType = RelationType(strings.ToUpper(strings.TrimSpace(string(t.RelationType))))
	t.SourceName = CanonicalName(t.SourceName)
	t.TargetName = CanonicalName(t.TargetName)
	t.Evidence = strings.TrimSpace(t.Evidence)
	if t.SourceName == "" || t.TargetName == "" || t.RelationType == "" || t.SourceType == "" || t.TargetType == "" {
		return Triple{}, false
	}
	if !isEntityType(t.SourceType) || !isEntityType(t.TargetType) || !isRelationType(t.RelationType) {
		return Triple{}, false
	}
	if t.Confidence < 0 {
		t.Confidence = 0
	}
	if t.Confidence > 1 {
		t.Confidence = 1
	}
	return t, true
}

func isEntityType(x EntityType) bool {
	switch x {
	case EntityPaper, EntityAuthor, EntityMethod, EntityDataset, EntityTask, EntityMetric, EntityOrganization:
		return true
	default:
		return false
	}
}

func isRelationType(x RelationType) bool {
	switch x {
	case RelCites, RelProposes, RelBasedOn, RelExtends, RelOutperforms, RelEvaluatedOn, RelAuthoredBy, RelImplements, RelUsesDataset:
		return true
	default:
		return false
	}
}
