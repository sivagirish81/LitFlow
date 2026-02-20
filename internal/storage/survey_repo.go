package storage

import (
	"context"
	"encoding/json"
	"fmt"
)

type SurveyRepo struct {
	db *DB
}

func NewSurveyRepo(db *DB) *SurveyRepo {
	return &SurveyRepo{db: db}
}

func (r *SurveyRepo) CreateRun(ctx context.Context, surveyRunID, corpusID string, topics, questions []string) error {
	topicJSON, _ := json.Marshal(topics)
	questionJSON, _ := json.Marshal(questions)
	_, err := r.db.Pool.Exec(ctx, `
INSERT INTO survey_runs (survey_run_id, corpus_id, topics, questions, status)
VALUES ($1, $2, $3::jsonb, $4::jsonb, 'pending')`, surveyRunID, corpusID, string(topicJSON), string(questionJSON))
	if err != nil {
		return fmt.Errorf("create survey run: %w", err)
	}
	return nil
}

func (r *SurveyRepo) UpdateRunStatus(ctx context.Context, surveyRunID, status, outPath string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE survey_runs SET status=$2, out_path=NULLIF($3,'' ) WHERE survey_run_id=$1`, surveyRunID, status, outPath)
	if err != nil {
		return fmt.Errorf("update survey run: %w", err)
	}
	return nil
}

func (r *SurveyRepo) GetRunPath(ctx context.Context, surveyRunID string) (string, string, error) {
	var outPath string
	var status string
	if err := r.db.Pool.QueryRow(ctx, `SELECT COALESCE(out_path,''), status FROM survey_runs WHERE survey_run_id=$1`, surveyRunID).Scan(&outPath, &status); err != nil {
		return "", "", fmt.Errorf("get survey run: %w", err)
	}
	return outPath, status, nil
}
