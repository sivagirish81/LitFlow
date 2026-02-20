package activities

type UpsertTopicGraphInput struct {
	CorpusID string  `json:"corpus_id"`
	Topic    string  `json:"topic"`
	PaperID  string  `json:"paper_id"`
	Title    string  `json:"title"`
	ChunkID  string  `json:"chunk_id"`
	Score    float64 `json:"score"`
}
