package activities

import "litflow/internal/graph"

type KGPaperInput struct {
	CorpusID string `json:"corpus_id"`
	PaperID  string `json:"paper_id"`
}

type KGPaperChunk struct {
	ChunkID string `json:"chunk_id"`
	Text    string `json:"text"`
}

type ListPaperChunksOutput struct {
	Title  string         `json:"title"`
	Chunks []KGPaperChunk `json:"chunks"`
}

type KGTripleRecord struct {
	SourceType   string  `json:"source_type"`
	SourceName   string  `json:"source_name"`
	RelationType string  `json:"relation_type"`
	TargetType   string  `json:"target_type"`
	TargetName   string  `json:"target_name"`
	Evidence     string  `json:"evidence"`
	Confidence   float64 `json:"confidence"`
	ChunkID      string  `json:"chunk_id"`
}

type UpsertKGTriplesInput struct {
	CorpusID     string           `json:"corpus_id"`
	PaperID      string           `json:"paper_id"`
	PromptHash   string           `json:"prompt_hash"`
	ModelVersion string           `json:"model_version"`
	Triples      []KGTripleRecord `json:"triples"`
}

type MarkKGPaperRunInput struct {
	CorpusID     string `json:"corpus_id"`
	PaperID      string `json:"paper_id"`
	PromptHash   string `json:"prompt_hash"`
	ModelVersion string `json:"model_version"`
	Status       string `json:"status"`
	TripleCount  int    `json:"triple_count"`
	LastError    string `json:"last_error"`
}

func ToKGRecord(t graph.Triple, chunkID string) KGTripleRecord {
	return KGTripleRecord{
		SourceType:   string(t.SourceType),
		SourceName:   t.SourceName,
		RelationType: string(t.RelationType),
		TargetType:   string(t.TargetType),
		TargetName:   t.TargetName,
		Evidence:     t.Evidence,
		Confidence:   t.Confidence,
		ChunkID:      chunkID,
	}
}
