package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type KGTripleInput struct {
	CorpusID     string
	PaperID      string
	PromptHash   string
	ModelVersion string
	SourceType   string
	SourceName   string
	RelationType string
	TargetType   string
	TargetName   string
	ChunkID      string
	Evidence     string
	Confidence   float64
}

type KGRunRecord struct {
	CorpusID     string
	PaperID      string
	PromptHash   string
	ModelVersion string
	Status       string
	TripleCount  int
	LastError    string
}

type LineageEdge struct {
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name"`
	EdgeType   string `json:"edge_type"`
	Depth      int    `json:"depth"`
}

func (r *GraphRepo) UpsertKGRun(ctx context.Context, in KGRunRecord) error {
	if err := r.ensureKGSchema(ctx); err != nil {
		return err
	}
	_, err := r.db.Pool.Exec(ctx, `
INSERT INTO kg_paper_runs(corpus_id, paper_id, prompt_hash, model_version, status, triple_count, last_error, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7,''), NOW())
ON CONFLICT (corpus_id, paper_id, prompt_hash, model_version)
DO UPDATE SET status = EXCLUDED.status, triple_count = EXCLUDED.triple_count, last_error = EXCLUDED.last_error, updated_at = NOW()`,
		in.CorpusID, in.PaperID, in.PromptHash, in.ModelVersion, in.Status, in.TripleCount, in.LastError)
	if err != nil {
		return fmt.Errorf("upsert kg run: %w", err)
	}
	return nil
}

func (r *GraphRepo) UpsertKGTriples(ctx context.Context, triples []KGTripleInput) error {
	if len(triples) == 0 {
		return nil
	}
	if err := r.ensureKGSchema(ctx); err != nil {
		return err
	}
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin kg triples tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, t := range triples {
		srcNodeID := fmt.Sprintf("%s:%s:%s", strings.ToLower(t.SourceType), t.CorpusID, slug(t.SourceName))
		dstNodeID := fmt.Sprintf("%s:%s:%s", strings.ToLower(t.TargetType), t.CorpusID, slug(t.TargetName))
		edgeID := fmt.Sprintf("edge:%s:%s:%s:%s", t.CorpusID, srcNodeID, dstNodeID, strings.ToUpper(t.RelationType))

		_, err = tx.Exec(ctx, `
INSERT INTO graph_nodes(node_id, corpus_id, node_type, label, payload)
VALUES ($1, $2::uuid, $3, $4, jsonb_build_object('canonical_name', $4::text, 'aliases', jsonb_build_array($5::text)))
ON CONFLICT (node_id)
DO UPDATE SET payload = jsonb_set(
  graph_nodes.payload,
  '{aliases}',
  (
    SELECT to_jsonb(ARRAY(
      SELECT DISTINCT x
      FROM jsonb_array_elements_text(COALESCE(graph_nodes.payload->'aliases','[]'::jsonb) || to_jsonb(ARRAY[$5::text])) AS x
    ))
  )
)`, srcNodeID, t.CorpusID, strings.ToLower(t.SourceType), t.SourceName, t.SourceName)
		if err != nil {
			return fmt.Errorf("upsert source node: %w", err)
		}

		_, err = tx.Exec(ctx, `
INSERT INTO graph_nodes(node_id, corpus_id, node_type, label, payload)
VALUES ($1, $2::uuid, $3, $4, jsonb_build_object('canonical_name', $4::text, 'aliases', jsonb_build_array($5::text)))
ON CONFLICT (node_id)
DO UPDATE SET payload = jsonb_set(
  graph_nodes.payload,
  '{aliases}',
  (
    SELECT to_jsonb(ARRAY(
      SELECT DISTINCT x
      FROM jsonb_array_elements_text(COALESCE(graph_nodes.payload->'aliases','[]'::jsonb) || to_jsonb(ARRAY[$5::text])) AS x
    ))
  )
)`, dstNodeID, t.CorpusID, strings.ToLower(t.TargetType), t.TargetName, t.TargetName)
		if err != nil {
			return fmt.Errorf("upsert target node: %w", err)
		}

		prov, _ := json.Marshal([]map[string]any{{
			"paper_id":      t.PaperID,
			"chunk_id":      t.ChunkID,
			"evidence":      t.Evidence,
			"confidence":    t.Confidence,
			"prompt_hash":   t.PromptHash,
			"model_version": t.ModelVersion,
		}})

		_, err = tx.Exec(ctx, `
INSERT INTO graph_edges(edge_id, corpus_id, source_node_id, target_node_id, edge_type, weight, payload)
VALUES ($1, $2::uuid, $3, $4, $5, $6, jsonb_build_object('support_count', 1, 'provenance', $7::jsonb, 'model_version', $8::text, 'prompt_hash', $9::text))
ON CONFLICT (corpus_id, source_node_id, target_node_id, edge_type)
DO UPDATE SET
  weight = GREATEST(graph_edges.weight, EXCLUDED.weight),
  payload = jsonb_build_object(
    'support_count', COALESCE((graph_edges.payload->>'support_count')::int, 0) + 1,
    'provenance', COALESCE(graph_edges.payload->'provenance','[]'::jsonb) || COALESCE(EXCLUDED.payload->'provenance','[]'::jsonb),
    'model_version', EXCLUDED.payload->>'model_version',
    'prompt_hash', EXCLUDED.payload->>'prompt_hash'
  )`,
			edgeID, t.CorpusID, srcNodeID, dstNodeID, strings.ToUpper(t.RelationType), t.Confidence, string(prov), t.ModelVersion, t.PromptHash)
		if err != nil {
			return fmt.Errorf("upsert kg edge: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit kg triples tx: %w", err)
	}
	return nil
}

func (r *GraphRepo) GetMethodLineage(ctx context.Context, corpusID, method string) ([]GraphNode, []LineageEdge, error) {
	root := fmt.Sprintf("method:%s:%s", corpusID, slug(method))
	rows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE lineage AS (
  SELECT e.source_node_id, e.target_node_id, e.edge_type, 1 AS depth
  FROM graph_edges e
  WHERE e.corpus_id = $1::uuid
    AND e.target_node_id = $2
    AND e.edge_type IN ('EXTENDS','BASED_ON')
  UNION ALL
  SELECT e.source_node_id, e.target_node_id, e.edge_type, l.depth + 1
  FROM graph_edges e
  JOIN lineage l ON e.target_node_id = l.source_node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND l.depth < 8
)
SELECT l.source_node_id, ns.label, l.target_node_id, nt.label, l.edge_type, l.depth
FROM lineage l
JOIN graph_nodes ns ON ns.node_id = l.source_node_id
JOIN graph_nodes nt ON nt.node_id = l.target_node_id
ORDER BY l.depth ASC`, corpusID, root)
	if err != nil {
		return nil, nil, fmt.Errorf("query lineage: %w", err)
	}
	defer rows.Close()
	nodes := map[string]GraphNode{}
	edges := make([]LineageEdge, 0)
	for rows.Next() {
		var e LineageEdge
		if err := rows.Scan(&e.SourceID, &e.SourceName, &e.TargetID, &e.TargetName, &e.EdgeType, &e.Depth); err != nil {
			return nil, nil, fmt.Errorf("scan lineage: %w", err)
		}
		edges = append(edges, e)
		nodes[e.SourceID] = GraphNode{NodeID: e.SourceID, NodeType: strings.SplitN(e.SourceID, ":", 2)[0], Label: e.SourceName}
		nodes[e.TargetID] = GraphNode{NodeID: e.TargetID, NodeType: strings.SplitN(e.TargetID, ":", 2)[0], Label: e.TargetName}
	}
	outNodes := make([]GraphNode, 0, len(nodes))
	for _, n := range nodes {
		outNodes = append(outNodes, n)
	}
	return outNodes, edges, rows.Err()
}

func (r *GraphRepo) QueryCypher(ctx context.Context, _ string, _ string) (map[string]any, error) {
	return nil, fmt.Errorf("cypher query requires neo4j store; postgres graph store only supports lineage and graph APIs")
}

func QueryCypherNeo4jHTTP(ctx context.Context, cypher string) (map[string]any, error) {
	base := strings.TrimSpace(os.Getenv("LITFLOW_NEO4J_HTTP_URL"))
	if base == "" {
		return nil, fmt.Errorf("neo4j is not configured")
	}
	user := strings.TrimSpace(os.Getenv("LITFLOW_NEO4J_USER"))
	pass := strings.TrimSpace(os.Getenv("LITFLOW_NEO4J_PASSWORD"))
	db := strings.TrimSpace(os.Getenv("LITFLOW_NEO4J_DATABASE"))
	if db == "" {
		db = "neo4j"
	}
	body, _ := json.Marshal(map[string]any{
		"statements": []map[string]any{{
			"statement":          cypher,
			"resultDataContents": []string{"row"},
			"includeStats":       false,
		}},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/db/"+db+"/tx/commit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("neo4j request failed: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("neo4j query error %d: %s", resp.StatusCode, string(b))
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, fmt.Errorf("decode neo4j response: %w", err)
	}
	return parsed, nil
}

func slug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func (r *GraphRepo) ensureKGSchema(ctx context.Context) error {
	r.kgSchemaMu.Lock()
	defer r.kgSchemaMu.Unlock()

	if r.kgSchemaPrepared {
		return nil
	}

	// Keep KG migrations resilient even if the operator forgot to run `make migrate`.
	ddl := `
CREATE TABLE IF NOT EXISTS kg_paper_runs (
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  paper_id TEXT NOT NULL REFERENCES papers(paper_id) ON DELETE CASCADE,
  prompt_hash TEXT NOT NULL,
  model_version TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('running','completed','failed')),
  triple_count INT NOT NULL DEFAULT 0,
  last_error TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (corpus_id, paper_id, prompt_hash, model_version)
);

CREATE INDEX IF NOT EXISTS idx_kg_runs_corpus ON kg_paper_runs(corpus_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_edges_type ON graph_edges(corpus_id, edge_type);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_type ON graph_nodes(corpus_id, node_type);

ALTER TABLE graph_nodes DROP CONSTRAINT IF EXISTS graph_nodes_node_type_check;
ALTER TABLE graph_nodes
  ADD CONSTRAINT graph_nodes_node_type_check
  CHECK (node_type IN ('paper','topic','entity','author','method','dataset','task','metric','organization'));
`
	if _, err := r.db.Pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("ensure kg schema: %w", err)
	}
	r.kgSchemaPrepared = true
	return nil
}
