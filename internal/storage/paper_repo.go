package storage

import (
	"context"
	"fmt"

	"litflow/internal/models"
)

type PaperRepo struct {
	db *DB
}

func NewPaperRepo(db *DB) *PaperRepo {
	return &PaperRepo{db: db}
}

func (r *PaperRepo) UpsertPaper(ctx context.Context, p models.Paper) error {
	_, err := r.db.Pool.Exec(ctx, `
INSERT INTO papers (paper_id, corpus_id, filename, title, authors, year, abstract, status, fail_reason)
VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), $6, NULLIF($7,''), $8, NULLIF($9,''))
ON CONFLICT (paper_id)
DO UPDATE SET
  corpus_id = EXCLUDED.corpus_id,
  filename = EXCLUDED.filename,
  title = COALESCE(EXCLUDED.title, papers.title),
  authors = COALESCE(EXCLUDED.authors, papers.authors),
  year = COALESCE(EXCLUDED.year, papers.year),
  abstract = COALESCE(EXCLUDED.abstract, papers.abstract),
  status = EXCLUDED.status,
  fail_reason = EXCLUDED.fail_reason,
  updated_at = NOW()`,
		p.PaperID, p.CorpusID, p.Filename, p.Title, p.Authors, p.Year, p.Abstract, p.Status, p.FailReason,
	)
	if err != nil {
		return fmt.Errorf("upsert paper: %w", err)
	}
	return nil
}

func (r *PaperRepo) UpdatePaperStatus(ctx context.Context, paperID, status, failReason string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE papers SET status=$2, fail_reason=NULLIF($3,''), updated_at=NOW() WHERE paper_id=$1`, paperID, status, failReason)
	if err != nil {
		return fmt.Errorf("update paper status: %w", err)
	}
	return nil
}

func (r *PaperRepo) ListPapersByCorpus(ctx context.Context, corpusID string) ([]models.Paper, error) {
	rows, err := r.db.Pool.Query(ctx, `
SELECT paper_id, corpus_id::text, filename, COALESCE(title,''), COALESCE(authors,''), year,
       COALESCE(abstract,''), status, COALESCE(fail_reason,''), created_at, updated_at
FROM papers
WHERE corpus_id=$1
ORDER BY created_at DESC`, corpusID)
	if err != nil {
		return nil, fmt.Errorf("list papers: %w", err)
	}
	defer rows.Close()

	out := make([]models.Paper, 0)
	for rows.Next() {
		var p models.Paper
		if err := rows.Scan(&p.PaperID, &p.CorpusID, &p.Filename, &p.Title, &p.Authors, &p.Year, &p.Abstract, &p.Status, &p.FailReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan paper: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate papers: %w", err)
	}
	return out, nil
}

func (r *PaperRepo) ListFailedPapers(ctx context.Context, corpusID string) ([]models.Paper, error) {
	rows, err := r.db.Pool.Query(ctx, `
SELECT paper_id, corpus_id::text, filename, COALESCE(title,''), COALESCE(authors,''), year,
       COALESCE(abstract,''), status, COALESCE(fail_reason,''), created_at, updated_at
FROM papers
WHERE corpus_id=$1 AND status='failed'
ORDER BY updated_at DESC`, corpusID)
	if err != nil {
		return nil, fmt.Errorf("list failed papers: %w", err)
	}
	defer rows.Close()
	out := make([]models.Paper, 0)
	for rows.Next() {
		var p models.Paper
		if err := rows.Scan(&p.PaperID, &p.CorpusID, &p.Filename, &p.Title, &p.Authors, &p.Year, &p.Abstract, &p.Status, &p.FailReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan failed paper: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PaperRepo) GetPaperByID(ctx context.Context, corpusID, paperID string) (models.Paper, error) {
	var p models.Paper
	err := r.db.Pool.QueryRow(ctx, `
SELECT paper_id, corpus_id::text, filename, COALESCE(title,''), COALESCE(authors,''), year,
       COALESCE(abstract,''), status, COALESCE(fail_reason,''), created_at, updated_at
FROM papers
WHERE corpus_id=$1 AND paper_id=$2`, corpusID, paperID).
		Scan(&p.PaperID, &p.CorpusID, &p.Filename, &p.Title, &p.Authors, &p.Year, &p.Abstract, &p.Status, &p.FailReason, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return models.Paper{}, fmt.Errorf("get paper by id: %w", err)
	}
	return p, nil
}

func (r *PaperRepo) ListPapersByIDs(ctx context.Context, corpusID string, paperIDs []string) ([]models.Paper, error) {
	if len(paperIDs) == 0 {
		return []models.Paper{}, nil
	}
	rows, err := r.db.Pool.Query(ctx, `
SELECT paper_id, corpus_id::text, filename, COALESCE(title,''), COALESCE(authors,''), year,
       COALESCE(abstract,''), status, COALESCE(fail_reason,''), created_at, updated_at
FROM papers
WHERE corpus_id=$1 AND paper_id = ANY($2)
ORDER BY created_at DESC`, corpusID, paperIDs)
	if err != nil {
		return nil, fmt.Errorf("list papers by ids: %w", err)
	}
	defer rows.Close()

	out := make([]models.Paper, 0, len(paperIDs))
	for rows.Next() {
		var p models.Paper
		if err := rows.Scan(&p.PaperID, &p.CorpusID, &p.Filename, &p.Title, &p.Authors, &p.Year, &p.Abstract, &p.Status, &p.FailReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan paper by id: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate papers by ids: %w", err)
	}
	return out, nil
}
