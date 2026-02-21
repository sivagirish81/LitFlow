ALTER TABLE graph_nodes
  DROP CONSTRAINT IF EXISTS graph_nodes_node_type_check;

ALTER TABLE graph_nodes
  ADD CONSTRAINT graph_nodes_node_type_check
  CHECK (node_type IN ('paper','topic','entity','author','method','dataset','task','metric','organization'));

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
