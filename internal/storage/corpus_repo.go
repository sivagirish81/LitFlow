package storage

import (
	"context"
	"fmt"

	"litflow/internal/models"
)

type CorpusRepo struct {
	db *DB
}

func NewCorpusRepo(db *DB) *CorpusRepo {
	return &CorpusRepo{db: db}
}

func (r *CorpusRepo) CreateCorpus(ctx context.Context, corpus models.Corpus) error {
	_, err := r.db.Pool.Exec(ctx, `INSERT INTO corpora (corpus_id, name) VALUES ($1, $2)`, corpus.CorpusID, corpus.Name)
	if err != nil {
		return fmt.Errorf("insert corpus: %w", err)
	}
	return nil
}

func (r *CorpusRepo) ListCorpora(ctx context.Context) ([]models.Corpus, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT corpus_id::text, name, created_at FROM corpora ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list corpora: %w", err)
	}
	defer rows.Close()

	out := make([]models.Corpus, 0)
	for rows.Next() {
		var c models.Corpus
		if err := rows.Scan(&c.CorpusID, &c.Name, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan corpus: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate corpora: %w", err)
	}
	return out, nil
}
