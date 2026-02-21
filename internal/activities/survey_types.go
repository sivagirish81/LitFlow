package activities

type EmbedQueryInput struct {
	Operation     string `json:"operation"`
	Text          string `json:"text"`
	ProviderIndex int    `json:"provider_index"`
}

type EmbedQueryOutput struct {
	Vector       []float32 `json:"vector"`
	ProviderName string    `json:"provider_name"`
	Model        string    `json:"model"`
}

type SearchChunksInput struct {
	CorpusID         string    `json:"corpus_id"`
	QueryVec         []float32 `json:"query_vec"`
	TopK             int       `json:"top_k"`
	EmbeddingVersion string    `json:"embedding_version,omitempty"`
}

type SearchChunk struct {
	PaperID string  `json:"paper_id"`
	Title   string  `json:"title"`
	ChunkID string  `json:"chunk_id"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Text    string  `json:"text"`
}

type SearchChunksOutput struct {
	Results []SearchChunk `json:"results"`
}

type WriteSurveyReportInput struct {
	CorpusID    string `json:"corpus_id"`
	SurveyRunID string `json:"survey_run_id"`
	Report      string `json:"report"`
}

type WriteSurveyReportOutput struct {
	OutPath string `json:"out_path"`
}

type UpdateSurveyRunInput struct {
	SurveyRunID string `json:"survey_run_id"`
	Status      string `json:"status"`
	OutPath     string `json:"out_path"`
}
