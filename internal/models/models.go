package models

import "time"

type Corpus struct {
	CorpusID  string    `json:"corpus_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Paper struct {
	PaperID    string    `json:"paper_id"`
	CorpusID   string    `json:"corpus_id"`
	Filename   string    `json:"filename"`
	Title      string    `json:"title,omitempty"`
	Authors    string    `json:"authors,omitempty"`
	Year       *int      `json:"year,omitempty"`
	Abstract   string    `json:"abstract,omitempty"`
	Status     string    `json:"status"`
	FailReason string    `json:"fail_reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Chunk struct {
	ChunkID          string    `json:"chunk_id"`
	PaperID          string    `json:"paper_id"`
	CorpusID         string    `json:"corpus_id"`
	ChunkIndex       int       `json:"chunk_index"`
	Text             string    `json:"text"`
	Section          string    `json:"section,omitempty"`
	PageStart        *int      `json:"page_start,omitempty"`
	PageEnd          *int      `json:"page_end,omitempty"`
	EmbeddingVersion string    `json:"embedding_version"`
	CreatedAt        time.Time `json:"created_at"`
}

type ChunkResult struct {
	PaperID   string  `json:"paper_id"`
	Title     string  `json:"title"`
	Filename  string  `json:"filename"`
	ChunkID   string  `json:"chunk_id"`
	Snippet   string  `json:"snippet"`
	Score     float64 `json:"score"`
	ChunkText string  `json:"chunk_text,omitempty"`
}
