package graph

type EntityType string

const (
	EntityPaper        EntityType = "paper"
	EntityAuthor       EntityType = "author"
	EntityMethod       EntityType = "method"
	EntityDataset      EntityType = "dataset"
	EntityTask         EntityType = "task"
	EntityMetric       EntityType = "metric"
	EntityOrganization EntityType = "organization"
)

type RelationType string

const (
	RelCites       RelationType = "CITES"
	RelProposes    RelationType = "PROPOSES"
	RelBasedOn     RelationType = "BASED_ON"
	RelExtends     RelationType = "EXTENDS"
	RelOutperforms RelationType = "OUTPERFORMS"
	RelEvaluatedOn RelationType = "EVALUATED_ON"
	RelAuthoredBy  RelationType = "AUTHORED_BY"
	RelImplements  RelationType = "IMPLEMENTS"
	RelUsesDataset RelationType = "USES_DATASET"
)

type Triple struct {
	SourceType   EntityType   `json:"source_type"`
	SourceName   string       `json:"source_name"`
	RelationType RelationType `json:"relation_type"`
	TargetType   EntityType   `json:"target_type"`
	TargetName   string       `json:"target_name"`
	Evidence     string       `json:"evidence"`
	Confidence   float64      `json:"confidence"`
}
