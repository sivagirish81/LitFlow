package vector

import (
	"context"
	"fmt"
	"strings"

	"litflow/internal/models"

	"github.com/jackc/pgx/v5"
)

type SearchFilters struct {
	PaperIDs         []string
	EmbeddingVersion string
}

type Searcher struct {
	q Queryer
}

type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func NewSearcher(q Queryer) *Searcher {
	return &Searcher{q: q}
}

func (s *Searcher) SearchChunks(ctx context.Context, corpusID string, queryVec []float32, topK int, filters SearchFilters) ([]models.ChunkResult, error) {
	if topK <= 0 {
		topK = 8
	}
	vecLiteral := ToLiteral(queryVec)
	args := []any{corpusID, vecLiteral, topK}

	filterSQL := ""
	if len(filters.PaperIDs) > 0 {
		filterSQL = " AND c.paper_id = ANY($4)"
		args = append(args, filters.PaperIDs)
	}
	if strings.TrimSpace(filters.EmbeddingVersion) != "" {
		if len(args) == 3 {
			filterSQL += " AND c.embedding_version = $4"
			args = append(args, filters.EmbeddingVersion)
		} else {
			filterSQL += " AND c.embedding_version = $5"
			args = append(args, filters.EmbeddingVersion)
		}
	}

	query := `
SELECT c.paper_id,
       COALESCE(p.title, p.filename) AS title,
       p.filename,
       c.chunk_id,
       LEFT(c.text, 420) AS snippet,
       1 - (c.embedding <=> $2::vector) AS score,
       c.text
FROM chunks c
JOIN papers p ON p.paper_id = c.paper_id
WHERE c.corpus_id = $1
  AND c.embedding IS NOT NULL` + filterSQL + `
ORDER BY c.embedding <=> $2::vector
LIMIT $3`

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query vector search: %w", err)
	}
	defer rows.Close()

	results := make([]models.ChunkResult, 0, topK)
	for rows.Next() {
		var r models.ChunkResult
		if err := rows.Scan(&r.PaperID, &r.Title, &r.Filename, &r.ChunkID, &r.Snippet, &r.Score, &r.ChunkText); err != nil {
			return nil, fmt.Errorf("scan chunk result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search rows: %w", err)
	}
	return results, nil
}

func ToLiteral(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, x := range v {
		parts = append(parts, fmt.Sprintf("%f", x))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
