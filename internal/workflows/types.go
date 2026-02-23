package workflows

type CorpusIngestInput struct {
	CorpusID              string `json:"corpus_id"`
	InputDir              string `json:"input_dir"`
	MaxConcurrentChildren int    `json:"max_concurrent_children"`
	EmbedProviders        int    `json:"embed_providers"`
	CooldownSeconds       int    `json:"cooldown_seconds"`
	ChunkVersion          string `json:"chunk_version"`
	EmbedVersion          string `json:"embed_version"`
}

type PaperProcessInput struct {
	CorpusID                    string `json:"corpus_id"`
	PaperPath                   string `json:"paper_path"`
	ChunkSize                   int    `json:"chunk_size"`
	ChunkOverlap                int    `json:"chunk_overlap"`
	ChunkVersion                string `json:"chunk_version"`
	EmbedVersion                string `json:"embed_version"`
	EmbedProviders              int    `json:"embed_providers"`
	PreferredEmbedProviderIndex int    `json:"preferred_embed_provider_index"`
	StrictEmbedProvider         bool   `json:"strict_embed_provider"`
	CooldownSeconds             int    `json:"cooldown_seconds"`
}

type SurveyBuildInput struct {
	SurveyRunID     string   `json:"survey_run_id"`
	CorpusID        string   `json:"corpus_id"`
	Prompt          string   `json:"prompt,omitempty"`
	Topics          []string `json:"topics"`
	Questions       []string `json:"questions"`
	OutputFormat    string   `json:"output_format,omitempty"`
	RetrievalTopK   int      `json:"retrieval_top_k,omitempty"`
	EmbedProviders  int      `json:"embed_providers"`
	LLMProviders    int      `json:"llm_providers"`
	LLMProviderRefs []string `json:"llm_provider_refs,omitempty"`
	CooldownSeconds int      `json:"cooldown_seconds"`
	EmbedVersion    string   `json:"embed_version"`
}

type BackfillInput struct {
	CorpusID                    string   `json:"corpus_id"`
	Mode                        string   `json:"mode"`
	SurveyRunID                 string   `json:"survey_run_id,omitempty"`
	Topics                      []string `json:"topics,omitempty"`
	Questions                   []string `json:"questions,omitempty"`
	DataInRoot                  string   `json:"data_in_root,omitempty"`
	ChunkVersion                string   `json:"chunk_version,omitempty"`
	EmbedVersion                string   `json:"embed_version,omitempty"`
	EmbedProviders              int      `json:"embed_providers,omitempty"`
	PreferredEmbedProviderIndex int      `json:"preferred_embed_provider_index,omitempty"`
	StrictEmbedProvider         bool     `json:"strict_embed_provider,omitempty"`
	LLMProviders                int      `json:"llm_providers,omitempty"`
	LLMProviderRefs             []string `json:"llm_provider_refs,omitempty"`
	CooldownSeconds             int      `json:"cooldown_seconds,omitempty"`
}

type PaperStatus struct {
	PaperID     string            `json:"paper_id"`
	PaperPath   string            `json:"paper_path"`
	CurrentStep string            `json:"current_step"`
	Status      string            `json:"status"`
	FailReason  string            `json:"fail_reason,omitempty"`
	Providers   []string          `json:"providers_used"`
	RetryCounts map[string]int    `json:"retry_counts"`
	Steps       map[string]string `json:"steps"`
}

type CorpusIngestProgress struct {
	CorpusID      string            `json:"corpus_id"`
	Total         int               `json:"total"`
	Done          int               `json:"done"`
	Failed        int               `json:"failed"`
	PerPaper      map[string]string `json:"per_paper_status"`
	ChildWorkflow map[string]string `json:"child_workflow_ids,omitempty"`
}

type SurveyProgress struct {
	SurveyRunID string            `json:"survey_run_id"`
	CorpusID    string            `json:"corpus_id"`
	TotalTopics int               `json:"total_topics"`
	DoneTopics  int               `json:"done_topics"`
	TopicStatus map[string]string `json:"topic_status"`
}

type KGBackfillInput struct {
	CorpusID        string   `json:"corpus_id"`
	PromptVersion   string   `json:"prompt_version"`
	ModelVersion    string   `json:"model_version"`
	LLMProviders    int      `json:"llm_providers"`
	LLMProviderRefs []string `json:"llm_provider_refs,omitempty"`
	CooldownSeconds int      `json:"cooldown_seconds"`
	MaxConcurrent   int      `json:"max_concurrent"`
}

type KGExtractPaperInput struct {
	CorpusID        string   `json:"corpus_id"`
	PaperID         string   `json:"paper_id"`
	PromptVersion   string   `json:"prompt_version"`
	ModelVersion    string   `json:"model_version"`
	LLMProviders    int      `json:"llm_providers"`
	LLMProviderRefs []string `json:"llm_provider_refs,omitempty"`
	CooldownSeconds int      `json:"cooldown_seconds"`
}

type KGBackfillProgress struct {
	CorpusID string            `json:"corpus_id"`
	Total    int               `json:"total"`
	Done     int               `json:"done"`
	Failed   int               `json:"failed"`
	PerPaper map[string]string `json:"per_paper_status"`
}
