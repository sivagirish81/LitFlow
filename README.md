# LitFlow

LitFlow is an open-source **research intelligence platform** for literature ingestion, retrieval, survey generation, and knowledge graph analytics.

It processes real research PDFs into a durable corpus, supports citation-grounded Q&A, generates topic reports, and surfaces graph-driven insights for method lineage, competitive performance, dataset dominance, and trends.

## Key Features
- **Corpus ingestion from real PDFs** (no synthetic seed data)
- **Temporal-native orchestration** for long-running, resumable pipelines
- **RAG Q&A with citations**
- **Survey builder** with Markdown reports
- **Knowledge Graph + Research Intelligence dashboard**
- **Backfills/reprocessing** (retry failed, re-embed, regenerate)
- **Local-first stack** with free defaults (`mock`, local embeddings)


### Workflow-level provider failover (not just activity retries)
Provider switching is implemented in workflow code with deterministic policy:
- Quota exhausted: disable provider for cooldown window, switch immediately
- Rate-limited: bounded backoff, then short disable and switch
- Transient: bounded retries, then switch
- Context-too-long: reduce context and retry path
- Permanent errors: fail step with explicit reason

Failover state is tracked in workflow state (`disabledUntil`, retry counters), so behavior is durable and replay-safe.

### Operational advantages over non-workflow systems
- Fan-out/fan-in ingestion with child workflows and bounded concurrency
- Queryable progress (`GetProgress`, `GetPaperStatus`, `GetSurveyProgress`, KG status)
- Idempotent activities and reproducible reruns
- Safe backfills with versioned manifests
- Clear event history and retry/failover trace in Temporal UI

## Architecture
- `cmd/api` - Go API server (`:8080`)
- `cmd/worker` - Go Temporal worker
- `apps/web` - Next.js + Tailwind UI (`:3000`)
- `internal/workflows` - Temporal workflows (ingest, survey, backfill, KG)
- `internal/activities` - idempotent workflow activities
- `internal/providers` - LLM/embedding provider abstractions + parsing
- `internal/storage` - Postgres repos
- `internal/vector` - pgvector search
- `internal/graph` - KG extraction, parsing, normalization
- `migrations` - schema + pgvector + KG migrations
- `docker-compose.yml` - Temporal, Temporal UI, Postgres

## Local Stack
Docker services:
- Temporal server: `localhost:7233`
- Temporal UI: `http://localhost:8233`
- Postgres + pgvector: `localhost:5432`

App services:
- API: `http://localhost:8080`
- Web: `http://localhost:3000`

Data directories:
- Inbound PDFs: `./data/in/{corpusId}/...`
- Artifacts/reports/manifests: `./data/out/{corpusId}/...`

## Prerequisites
- Docker + Docker Compose
- Go (compatible with `go.mod`)
- Node.js 20+
- npm

## Quickstart
```bash
cp .env.example .env
make up
make migrate
```

Run in separate terminals:
```bash
make worker
make api
make web
```

Open:
- Web: `http://localhost:3000`
- Temporal UI: `http://localhost:8233`

Stop everything:
```bash
make down
```

## Make Targets
- `make up`
- `make down`
- `make migrate`
- `make api`
- `make worker`
- `make web`
- `make test`

## Configuration
Both API and worker auto-load `.env`.

Core provider settings:
- `LITFLOW_LLM_PROVIDERS="mock|openai:key1|groq:key2"`
- `LITFLOW_EMBED_PROVIDERS="mock|ollama:nomic|ollama:bge|openai:key1"`
- `LITFLOW_PROVIDER_COOLDOWN_SECONDS=900`

Embedding settings:
- `LITFLOW_EMBED_DIM=1536`
- `LITFLOW_EMBED_VERSION=v1`
- `LITFLOW_CHUNK_SIZE=1200`
- `LITFLOW_CHUNK_OVERLAP=200`

Frontend API base:
- `NEXT_PUBLIC_LITFLOW_API_BASE=http://localhost:8080`

Optional providers:
- OpenAI: `OPENAI_API_KEY` or aliased `LITFLOW_OPENAI_KEY_<ALIAS>`
- Groq: `GROQ_API_KEY` or aliased `LITFLOW_GROQ_KEY_<ALIAS>`
- Ollama embeddings:
  - `LITFLOW_OLLAMA_BASE_URL=http://localhost:11434`
  - `LITFLOW_OLLAMA_EMBED_MODEL_NOMIC=nomic-embed-text`
  - `LITFLOW_OLLAMA_EMBED_MODEL_BGE=bge-small-en-v1.5`

## Core Workflows
### `CorpusIngestWorkflow`
- Lists PDFs from corpus input directory
- Starts `PaperProcessWorkflow` children with concurrency limits
- Continues despite individual paper failures
- Exposes query: `GetProgress`
- Writes corpus summary artifact

### `PaperProcessWorkflow`
- Computes stable `paper_id`
- Extracts text (text PDFs only; no OCR)
- Chunks and embeds with provider failover
- Upserts chunks + embeddings idempotently
- Writes per-paper artifacts and status
- Exposes query: `GetPaperStatus`

### `SurveyBuildWorkflow`
- Retrieves relevant chunks per topic
- Generates outline + sections with failover
- Produces Markdown report + citations
- Exposes query: `GetSurveyProgress`

### `BackfillWorkflow`
- `RETRY_FAILED_PAPERS`
- `REEMBED_ALL_PAPERS`
- `REGENERATE_SURVEY`
- Emits versioned run manifest

### KG Workflows
- `KGBackfillWorkflow` (corpus-wide)
- `KGExtractPaperWorkflow` (single paper)

## Research Intelligence Dashboard
The Knowledge Graph page provides productized insights (not query-console-first UX):
- Overview
- Lineage Explorer
- Performance Matrix
- Dataset Dominance
- Trend Timeline
- Full Graph (optional deep view)

## Temporal Dashboard (Observability)
Open `http://localhost:8233`.

Recommended views:
- Filter by workflow type:
  - `CorpusIngestWorkflow`
  - `PaperProcessWorkflow`
  - `SurveyBuildWorkflow`
  - `BackfillWorkflow`
  - `KGBackfillWorkflow`
  - `KGExtractPaperWorkflow`
- Inspect event history to trace:
  - activity retries
  - provider failover transitions
  - cooldown/disable behavior
  - terminal failure causes

## Reliability Guarantees
- Idempotent DB upserts (`ON CONFLICT DO UPDATE`)
- Atomic artifact writes (temp + rename)
- Workflow-level provider failover policy
- Audit trail for provider calls (`llm_calls`)
- KG schema bootstrap protections for local drift

## Validation
```bash
go test ./...
go build ./...
cd apps/web && npm install && npm run build
```
