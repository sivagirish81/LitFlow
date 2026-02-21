package activities

type ComputePaperIDInput struct {
	PaperPath string `json:"paper_path"`
}

type ComputePaperIDOutput struct {
	PaperID string `json:"paper_id"`
}

type ListPDFsInput struct {
	InputDir string `json:"input_dir"`
}

type ListPDFsOutput struct {
	Paths []string `json:"paths"`
}

type WriteCorpusSummaryInput struct {
	CorpusID string         `json:"corpus_id"`
	Summary  map[string]any `json:"summary"`
}

type ListFailedPapersInput struct {
	CorpusID string `json:"corpus_id"`
}

type FailedPaper struct {
	PaperID  string `json:"paper_id"`
	Filename string `json:"filename"`
}

type ListFailedPapersOutput struct {
	Papers []FailedPaper `json:"papers"`
}

type ListCorpusPapersInput struct {
	CorpusID string `json:"corpus_id"`
}

type CorpusPaper struct {
	PaperID    string `json:"paper_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Title      string `json:"title,omitempty"`
	Authors    string `json:"authors,omitempty"`
	Year       int    `json:"year,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
}

type ListCorpusPapersOutput struct {
	Papers []CorpusPaper `json:"papers"`
}

type WriteRunManifestInput struct {
	CorpusID string         `json:"corpus_id"`
	RunID    string         `json:"run_id"`
	Manifest map[string]any `json:"manifest"`
}

type WriteRunManifestOutput struct {
	Path string `json:"path"`
}

type ExtractTextInput struct {
	PaperPath string `json:"paper_path"`
}

type ExtractTextOutput struct {
	Text string `json:"text"`
}

type ExtractMetadataInput struct {
	Text string `json:"text"`
}

type ExtractMetadataOutput struct {
	Title   string `json:"title"`
	Authors string `json:"authors"`
}

type ChunkTextInput struct {
	PaperID      string `json:"paper_id"`
	CorpusID     string `json:"corpus_id"`
	Text         string `json:"text"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	Version      string `json:"version"`
}

type ChunkItem struct {
	ChunkID    string `json:"chunk_id"`
	PaperID    string `json:"paper_id"`
	CorpusID   string `json:"corpus_id"`
	ChunkIndex int    `json:"chunk_index"`
	Text       string `json:"text"`
}

type ChunkTextOutput struct {
	Chunks []ChunkItem `json:"chunks"`
}

// UpsertChunksInput omits embeddings in M5.
type UpsertChunksInput struct {
	Chunks           []ChunkItem `json:"chunks"`
	Vectors          [][]float32 `json:"vectors,omitempty"`
	EmbeddingVersion string      `json:"embedding_version"`
}

type WritePaperArtifactsInput struct {
	CorpusID      string                 `json:"corpus_id"`
	PaperID       string                 `json:"paper_id"`
	Metadata      map[string]any         `json:"metadata"`
	Chunks        []ChunkItem            `json:"chunks"`
	ProcessingLog map[string]interface{} `json:"processing_log"`
}

type UpdatePaperStatusInput struct {
	PaperID    string `json:"paper_id"`
	CorpusID   string `json:"corpus_id"`
	Filename   string `json:"filename"`
	Title      string `json:"title"`
	Authors    string `json:"authors"`
	Status     string `json:"status"`
	FailReason string `json:"fail_reason"`
}

type EmbedChunksInput struct {
	Operation     string      `json:"operation"`
	CorpusID      string      `json:"corpus_id"`
	PaperID       string      `json:"paper_id"`
	ProviderIndex int         `json:"provider_index"`
	Input         []ChunkItem `json:"input"`
}

type EmbedChunksOutput struct {
	Vectors      [][]float32 `json:"vectors"`
	ProviderName string      `json:"provider_name"`
	Model        string      `json:"model"`
}

type LLMGenerateInput struct {
	Operation     string   `json:"operation"`
	CorpusID      string   `json:"corpus_id"`
	PaperID       string   `json:"paper_id"`
	Prompt        string   `json:"prompt"`
	Context       []string `json:"context"`
	ProviderIndex int      `json:"provider_index"`
}

type LLMGenerateOutput struct {
	Text         string `json:"text"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
}

type LogLLMCallInput struct {
	CallID       string `json:"call_id"`
	Operation    string `json:"operation"`
	CorpusID     string `json:"corpus_id"`
	PaperID      string `json:"paper_id"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	RequestID    string `json:"request_id"`
	Status       string `json:"status"`
	ErrorType    string `json:"error_type"`
}
