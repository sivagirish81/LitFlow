package storage

import (
	"context"
	"fmt"
)

type LLMCallRecord struct {
	CallID       string
	Operation    string
	CorpusID     string
	PaperID      string
	ProviderName string
	Model        string
	RequestID    string
	Status       string
	ErrorType    string
}

type LLMAuditRepo struct {
	db *DB
}

func NewLLMAuditRepo(db *DB) *LLMAuditRepo {
	return &LLMAuditRepo{db: db}
}

func (r *LLMAuditRepo) Insert(ctx context.Context, rec LLMCallRecord) error {
	_, err := r.db.Pool.Exec(ctx, `
INSERT INTO llm_calls(call_id, operation, corpus_id, paper_id, provider_name, model, request_id, status, error_type)
VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, NULLIF($3,'')::uuid, NULLIF($4,''), $5, $6, $7, $8, NULLIF($9,''))`,
		rec.CallID, rec.Operation, rec.CorpusID, rec.PaperID, rec.ProviderName, rec.Model, rec.RequestID, rec.Status, rec.ErrorType)
	if err != nil {
		return fmt.Errorf("insert llm call: %w", err)
	}
	return nil
}
