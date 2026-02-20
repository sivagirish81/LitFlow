# LitFlow

A durable AI-powered literature survey engine that reliably processes large research corpora, builds RAG indices, and generates structured, citation-backed literature reviews with full reproducibility and crash-safe execution.

## What it does
- Upload real PDFs into corpora
- Ingest PDFs via Temporal workflows (extract text, chunk, embed, upsert to Postgres+pgvector)
- Ask corpus questions with citation evidence
- Build topic-driven survey reports in Markdown
- Explore a knowledge graph (papers <-> topics)
- Run backfills (retry failed papers, regenerate surveys)

## Architecture
- `cmd/api`: Go HTTP API (`:8080`)
- `cmd/worker`: Go Temporal worker
- `apps/web`: Next.js + Tailwind UI (`:3000`)
- `docker-compose.yml`: Temporal server, Temporal UI, Postgres+pgvector
- Local files:
  - uploads: `./data/in/{corpusId}/...`
  - artifacts: `./data/out/{corpusId}/...`

## Prerequisites
- Docker + Docker Compose
- Go (toolchain compatible with `go.mod`)
- Node.js 20+
- npm

## Quickstart
Create local env file first:
```bash
cp .env.example .env
```

Edit `.env` and set API keys if needed.

Then start infrastructure:
```bash
make up
make migrate
```

Run services in separate terminals:
```bash
make worker
make api
make web
```

Open:
- Web app: http://localhost:3000
- Temporal UI: http://localhost:8233

Shutdown:
```bash
make down
```

## Make targets
- `make up`
- `make down`
- `make migrate`
- `make api`
- `make worker`
- `make web`
- `make test`

## Environment
LitFlow now auto-loads `.env` for API/worker startup (`cmd/api`, `cmd/worker`) and Docker Compose reads `.env` automatically.

Defaults are local-first and free:
- `LITFLOW_LLM_PROVIDERS="mock|openai:keyname1|openai:keyname2"`
- `LITFLOW_EMBED_PROVIDERS="mock|openai:keyname1|openai:keyname2"` (Groq is LLM-only)
- `LITFLOW_PROVIDER_COOLDOWN_SECONDS=900`
- `LITFLOW_EMBED_DIM=1536`
- `LITFLOW_CHUNK_SIZE=1200`
- `LITFLOW_CHUNK_OVERLAP=200`
- `NEXT_PUBLIC_LITFLOW_API_BASE=http://localhost:8080`
- `OPENAI_API_KEY=...` or alias keys like `LITFLOW_OPENAI_KEY_KEYNAME1=...`
- `GROQ_API_KEY=...` or alias keys like `LITFLOW_GROQ_KEY_KEYNAME1=...`
- `LITFLOW_GROQ_MODEL=llama-3.1-8b-instant`

Example using Groq LLM with free local embeddings:
- `LITFLOW_LLM_PROVIDERS="mock|groq:free1"`
- `LITFLOW_EMBED_PROVIDERS="mock"`

`mock` provider is deterministic and works without tokens.

## Temporal workflows
- `CorpusIngestWorkflow`
  - lists PDFs
  - fans out `PaperProcessWorkflow` child workflows with concurrency control
  - exposes `GetProgress` query
  - writes `./data/out/{corpusId}/corpus_summary.json`
- `PaperProcessWorkflow`
  - compute stable `paper_id`
  - extract text (no OCR)
  - chunk and embed
  - idempotent upsert
  - writes per-paper artifacts
  - exposes `GetPaperStatus`
- `SurveyBuildWorkflow`
  - per-topic retrieval
  - outline + section generation
  - citation list generation
  - writes `report.md`
  - exposes `GetSurveyProgress`
- `BackfillWorkflow`
  - `RETRY_FAILED_PAPERS`
  - `REGENERATE_SURVEY`
  - writes run manifest under `./data/out/{corpusId}/runs/{runId}/manifest.json`

## Reliability notes
- Upserts use `ON CONFLICT DO UPDATE`
- Artifact writes use temp file + atomic rename
- Text-only PDFs supported (no OCR in MVP)
- Provider call auditing recorded in `llm_calls`

## Verification
Validated locally:
```bash
go test ./...
go build ./...
cd apps/web && npm install && npm run build
```
