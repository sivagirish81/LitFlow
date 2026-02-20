package storage

import (
	"context"
	"fmt"
)

type ChunkRecord struct {
	ChunkID          string
	PaperID          string
	CorpusID         string
	ChunkIndex       int
	Text             string
	EmbeddingVersion string
	EmbeddingVector  *string
}

type ChunkRepo struct {
	db *DB
}

func NewChunkRepo(db *DB) *ChunkRepo {
	return &ChunkRepo{db: db}
}

func (r *ChunkRepo) UpsertChunks(ctx context.Context, chunks []ChunkRecord) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx upsert chunks: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, c := range chunks {
		_, err := tx.Exec(ctx, `
INSERT INTO chunks (chunk_id, paper_id, corpus_id, chunk_index, text, embedding_version, embedding)
VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7::text IS NULL THEN NULL ELSE $7::vector END)
ON CONFLICT (chunk_id)
DO UPDATE SET
  text = EXCLUDED.text,
  embedding_version = EXCLUDED.embedding_version,
  embedding = COALESCE(EXCLUDED.embedding, chunks.embedding)`,
			c.ChunkID, c.PaperID, c.CorpusID, c.ChunkIndex, c.Text, c.EmbeddingVersion, c.EmbeddingVector,
		)
		if err != nil {
			return fmt.Errorf("upsert chunk %s: %w", c.ChunkID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chunks tx: %w", err)
	}
	return nil
}
