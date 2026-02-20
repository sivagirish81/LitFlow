CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS corpora (
  corpus_id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS papers (
  paper_id TEXT PRIMARY KEY,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  title TEXT,
  authors TEXT,
  year INT,
  abstract TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','processed','failed')),
  fail_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_papers_corpus ON papers(corpus_id);
CREATE INDEX IF NOT EXISTS idx_papers_status ON papers(status);

CREATE TABLE IF NOT EXISTS chunks (
  chunk_id TEXT PRIMARY KEY,
  paper_id TEXT NOT NULL REFERENCES papers(paper_id) ON DELETE CASCADE,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  chunk_index INT NOT NULL,
  text TEXT NOT NULL,
  section TEXT,
  page_start INT,
  page_end INT,
  embedding vector(1536),
  embedding_version TEXT NOT NULL DEFAULT 'v1',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (paper_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_chunks_corpus ON chunks(corpus_id);
CREATE INDEX IF NOT EXISTS idx_chunks_paper ON chunks(paper_id);
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_ivfflat ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE TABLE IF NOT EXISTS llm_calls (
  call_id UUID PRIMARY KEY,
  operation TEXT NOT NULL,
  corpus_id UUID,
  paper_id TEXT,
  provider_name TEXT NOT NULL,
  model TEXT,
  request_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('ok','failed')),
  error_type TEXT CHECK (error_type IN ('quota','rate','transient','permanent','context')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_calls_corpus ON llm_calls(corpus_id);
CREATE INDEX IF NOT EXISTS idx_llm_calls_request_id ON llm_calls(request_id);

CREATE TABLE IF NOT EXISTS survey_runs (
  survey_run_id UUID PRIMARY KEY,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  topics JSONB NOT NULL DEFAULT '[]'::jsonb,
  questions JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending',
  out_path TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS graph_nodes (
  node_id TEXT PRIMARY KEY,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  node_type TEXT NOT NULL CHECK (node_type IN ('paper','topic','entity')),
  label TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS graph_edges (
  edge_id TEXT PRIMARY KEY,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  source_node_id TEXT NOT NULL REFERENCES graph_nodes(node_id) ON DELETE CASCADE,
  target_node_id TEXT NOT NULL REFERENCES graph_nodes(node_id) ON DELETE CASCADE,
  edge_type TEXT NOT NULL,
  weight DOUBLE PRECISION NOT NULL DEFAULT 0,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(corpus_id, source_node_id, target_node_id, edge_type)
);

CREATE TABLE IF NOT EXISTS backfill_runs (
  run_id UUID PRIMARY KEY,
  corpus_id UUID NOT NULL REFERENCES corpora(corpus_id) ON DELETE CASCADE,
  mode TEXT NOT NULL,
  manifest_path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
