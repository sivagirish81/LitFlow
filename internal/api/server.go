package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"litflow/internal/config"
	"litflow/internal/models"
	"litflow/internal/providers"
	"litflow/internal/storage"
	"litflow/internal/util"
	"litflow/internal/vector"
	"litflow/internal/workflows"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	tclient "go.temporal.io/sdk/client"
)

type Server struct {
	cfg        config.Config
	db         *storage.DB
	corpusRepo *storage.CorpusRepo
	paperRepo  *storage.PaperRepo
	surveyRepo *storage.SurveyRepo
	graphRepo  *storage.GraphRepo
	searcher   *vector.Searcher
	providers  *providers.Manager
	temporal   tclient.Client
}

type askCitation struct {
	RefID    string  `json:"ref_id"`
	PaperID  string  `json:"paper_id"`
	Title    string  `json:"title"`
	Filename string  `json:"filename,omitempty"`
	PaperURL string  `json:"paper_url,omitempty"`
	ChunkID  string  `json:"chunk_id"`
	Snippet  string  `json:"snippet"`
	Summary  string  `json:"summary,omitempty"`
	Score    float64 `json:"score"`
}

func NewServer(cfg config.Config) *Server {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := storage.NewDB(ctx, cfg.PostgresURL)
	if err != nil {
		panic(err)
	}
	pm, err := providers.NewManager(cfg)
	if err != nil {
		panic(err)
	}
	tc, err := tclient.Dial(tclient.Options{HostPort: cfg.TemporalAddress})
	if err != nil {
		panic(err)
	}
	return &Server{
		cfg:        cfg,
		db:         db,
		corpusRepo: storage.NewCorpusRepo(db),
		paperRepo:  storage.NewPaperRepo(db),
		surveyRepo: storage.NewSurveyRepo(db),
		graphRepo:  storage.NewGraphRepo(db),
		searcher:   vector.NewSearcher(db.Pool),
		providers:  pm,
		temporal:   tc,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/corpora", s.handleCorpora)
	mux.HandleFunc("/corpora/", s.handleCorporaScoped)
	mux.HandleFunc("/ask", s.handleAsk)
	mux.HandleFunc("/survey", s.handleSurvey)
	mux.HandleFunc("/survey/", s.handleSurveyScoped)
	return withCORS(mux)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCorpora(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		corpora, err := s.corpusRepo.ListCorpora(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"corpora": corpora})
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("name is required"))
			return
		}

		corpusID := uuid.NewString()
		corpus := models.Corpus{CorpusID: corpusID, Name: req.Name}
		if err := s.corpusRepo.CreateCorpus(r.Context(), corpus); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		if err := util.EnsureDir(filepath.Join(s.cfg.DataInRoot, corpusID)); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if err := util.EnsureDir(filepath.Join(s.cfg.DataOutRoot, corpusID)); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"corpus_id": corpusID, "name": req.Name})
	default:
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) handleCorporaScoped(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/corpora/"), "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	corpusID := parts[0]

	if len(parts) == 2 && parts[1] == "upload" {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		s.handleUpload(w, r, corpusID)
		return
	}

	if len(parts) == 2 && parts[1] == "papers" {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		papers, err := s.paperRepo.ListPapersByCorpus(r.Context(), corpusID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"papers": papers})
		return
	}
	if len(parts) == 4 && parts[1] == "papers" && parts[3] == "file" {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		paperID := parts[2]
		p, err := s.paperRepo.GetPaperByID(r.Context(), corpusID, paperID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		path := filepath.Join(s.cfg.DataInRoot, corpusID, filepath.Base(p.Filename))
		http.ServeFile(w, r, path)
		return
	}
	if len(parts) == 2 && parts[1] == "ingest" {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		wfID := "ingest-" + corpusID
		we, err := s.temporal.ExecuteWorkflow(r.Context(), tclient.StartWorkflowOptions{
			ID:                                       wfID,
			TaskQueue:                                s.cfg.TemporalTaskQueue,
			WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
			WorkflowExecutionErrorWhenAlreadyStarted: true,
		}, workflows.CorpusIngestWorkflow, workflows.CorpusIngestInput{
			CorpusID:              corpusID,
			InputDir:              filepath.Join(s.cfg.DataInRoot, corpusID),
			MaxConcurrentChildren: s.cfg.IngestMaxChildren,
			EmbedProviders:        s.providers.EmbedCount(),
			CooldownSeconds:       s.cfg.ProviderCooldownSecs,
		})
		if err != nil {
			writeErr(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"workflow_id": we.GetID(), "run_id": we.GetRunID()})
		return
	}
	if len(parts) == 2 && parts[1] == "progress" {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var prog workflows.CorpusIngestProgress
		resp, err := s.temporal.QueryWorkflow(r.Context(), "ingest-"+corpusID, "", workflows.QueryGetProgress)
		if err != nil {
			// Fallback to DB-derived progress when no active workflow query is available.
			papers, pErr := s.paperRepo.ListPapersByCorpus(r.Context(), corpusID)
			if pErr != nil {
				writeErr(w, http.StatusInternalServerError, pErr)
				return
			}
			per := make(map[string]string, len(papers))
			done := 0
			failed := 0
			for _, p := range papers {
				per[p.Filename] = p.Status
				if p.Status == "processed" {
					done++
				}
				if p.Status == "failed" {
					failed++
				}
			}
			writeJSON(w, http.StatusOK, workflows.CorpusIngestProgress{
				CorpusID: corpusID,
				Total:    len(papers),
				Done:     done,
				Failed:   failed,
				PerPaper: per,
			})
			return
		}
		if err := resp.Get(&prog); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, prog)
		return
	}
	if len(parts) == 2 && parts[1] == "graph" {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		nodes, edges, err := s.graphRepo.GetGraph(r.Context(), corpusID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
		return
	}

	writeErr(w, http.StatusNotFound, fmt.Errorf("not found"))
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request, corpusID string) {
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("parse multipart: %w", err))
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		if single, ok := firstSingleFile(r.MultipartForm.File); ok {
			files = append(files, single)
		}
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no files provided"))
		return
	}

	inDir := filepath.Join(s.cfg.DataInRoot, corpusID)
	if err := util.EnsureDir(inDir); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	type uploadResult struct {
		Filename string `json:"filename"`
		PaperID  string `json:"paper_id"`
	}
	out := make([]uploadResult, 0, len(files))

	for _, fh := range files {
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".pdf") {
			continue
		}
		paperID, savedPath, err := saveUploadedFile(inDir, fh)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if err := s.paperRepo.UpsertPaper(r.Context(), models.Paper{
			PaperID:  paperID,
			CorpusID: corpusID,
			Filename: filepath.Base(savedPath),
			Status:   "pending",
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, uploadResult{Filename: filepath.Base(savedPath), PaperID: paperID})
	}

	writeJSON(w, http.StatusOK, map[string]any{"uploaded": out})
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req struct {
		CorpusID string `json:"corpus_id"`
		Question string `json:"question"`
		TopK     int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
		return
	}
	req.CorpusID = strings.TrimSpace(req.CorpusID)
	req.Question = strings.TrimSpace(req.Question)
	if req.CorpusID == "" || req.Question == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("corpus_id and question are required"))
		return
	}
	if req.TopK <= 0 {
		req.TopK = 8
	}

	var (
		info providers.ProviderInfo
		err  error
	)
	embedOrders := s.providers.PreferredEmbedOrder()
	queryVectors := [][]float32(nil)
	for _, idx := range embedOrders {
		p, _ := s.providers.EmbedProviderByIndex(idx)
		queryVectors, info, err = p.Embed(r.Context(), providers.EmbedRequest{
			Operation: "ask_query_embed",
			Inputs:    []string{req.Question},
			Dimension: s.cfg.EmbedDim,
		})
		if err == nil && len(queryVectors) > 0 {
			break
		}
	}
	if err != nil || len(queryVectors) == 0 {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("embedding providers unavailable"))
		return
	}
	results, err := s.searcher.SearchChunks(r.Context(), req.CorpusID, queryVectors[0], req.TopK, vector.SearchFilters{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	citations := make([]askCitation, 0, len(results))
	contextSnippets := make([]string, 0, len(results))
	citationContexts := make([]string, 0, len(results))
	for i, r := range results {
		refID := fmt.Sprintf("C%d", i+1)
		displayTitle := util.DisplaySnippet(r.Title, 100)
		if displayTitle == "" {
			displayTitle = util.DisplaySnippet(r.Filename, 100)
		}
		snippet := util.DisplayEvidenceSnippet(r.ChunkText, req.Question, 420)
		if snippet == "" {
			snippet = util.DisplaySnippet(r.Snippet, 420)
		}
		contextText := util.DisplaySnippet(r.ChunkText, 1200)
		citations = append(citations, askCitation{
			RefID:    refID,
			PaperID:  r.PaperID,
			Title:    displayTitle,
			Filename: r.Filename,
			PaperURL: fmt.Sprintf("/corpora/%s/papers/%s/file", req.CorpusID, r.PaperID),
			ChunkID:  r.ChunkID,
			Snippet:  snippet,
			Score:    r.Score,
		})
		fullContext := fmt.Sprintf("%s | %s [%s]: %s", refID, displayTitle, r.ChunkID, contextText)
		contextSnippets = append(contextSnippets, fullContext)
		citationContexts = append(citationContexts, fullContext)
	}

	var (
		llmResp providers.GenerateResponse
		llmInfo providers.ProviderInfo
		llmErr  error
	)
	generate := func(op, prompt string, ctxSnippets []string) (providers.GenerateResponse, providers.ProviderInfo, error) {
		if groqProvider, groqRef, ok := s.providers.FindLLMProviderByName("groq"); ok {
			resp, info, err := groqProvider.Generate(r.Context(), providers.GenerateRequest{
				Operation: op,
				Prompt:    prompt,
				Context:   ctxSnippets,
			})
			info.Name = groqRef.Name
			return resp, info, err
		}
		var (
			resp providers.GenerateResponse
			info providers.ProviderInfo
			err  error
		)
		for _, idx := range s.providers.PreferredLLMOrder() {
			p, _ := s.providers.LLMProviderByIndex(idx)
			resp, info, err = p.Generate(r.Context(), providers.GenerateRequest{
				Operation: op,
				Prompt:    prompt,
				Context:   ctxSnippets,
			})
			if err == nil && strings.TrimSpace(resp.Text) != "" {
				return resp, info, nil
			}
		}
		return resp, info, err
	}

	prompt := "" +
		"Question: " + req.Question + "\n\n" +

		"You must answer using ONLY the provided evidence snippets.\n" +
		"Do NOT use outside knowledge.\n" +
		"If the snippets do not contain enough information, explicitly state what is missing.\n\n" +

		"Citation rules:\n" +
		"- Use citations like [C1], [C2], etc. whenever making a factual claim.\n" +
		"- Cite the snippet immediately after the sentence it supports.\n" +
		"- Multiple citations may be used together like [C1][C3] if needed.\n" +
		"- Do NOT cite anything not present in the provided snippets.\n\n" +

		"Answer guidelines:\n" +
		"- Write a clear, well-structured explanation in natural paragraphs.\n" +
		"- Bullet points are optional.\n" +
		"- Be specific: include definitions, numbers, experimental results, assumptions, constraints, and limitations when available.\n" +
		"- If snippets conflict, explain the disagreement and cite both.\n\n" +

		"Return markdown with this structure:\n" +
		"## Direct Answer\n" +
		"(Write a clear explanation. Bullets are optional.)\n\n" +
		"## Confidence\n" +
		"(State High/Medium/Low and briefly explain why, including evidence gaps.)\n\n" +

		"Evidence snippets (cite as [C#]):\n"
	llmResp, llmInfo, llmErr = generate("rag_answer", prompt, contextSnippets)
	if llmErr != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("generation failed: %w", llmErr))
		return
	}

	for i := range citations {
		c := citations[i]
		summaryPrompt := "Question: " + req.Question + "\n\n" +
			"Write exactly two short sentences:\n" +
			"1) what this citation supports for the question\n" +
			"2) one caveat or limitation.\n" +
			"Use plain language and do not include citation ids."
		sumResp, _, sumErr := generate("citation_summary", summaryPrompt, []string{citationContexts[i]})
		if sumErr != nil || strings.TrimSpace(sumResp.Text) == "" {
			citations[i].Summary = util.DisplayEvidenceSnippet(c.Snippet, req.Question, 240)
			continue
		}
		citations[i].Summary = util.DisplaySnippet(sumResp.Text, 260)
	}

	answer := strings.TrimSpace(llmResp.Text)
	if answer == "" {
		answer = fallbackExtractiveAnswer(citations)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"answer":          answer,
		"citations":       citations,
		"embed_provider":  info.Name,
		"embed_model":     info.Model,
		"llm_provider":    llmInfo.Name,
		"llm_model":       llmInfo.Model,
		"retrieved_count": len(citations),
	})
}

func fallbackExtractiveAnswer(citations []askCitation) string {
	if len(citations) == 0 {
		return "No relevant evidence was retrieved for this question."
	}
	lines := make([]string, 0, 6)
	lines = append(lines, "## Direct Answer")
	lines = append(lines, "- Retrieved evidence suggests the following:")
	limit := len(citations)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		title := citations[i].Title
		chunkID := citations[i].ChunkID
		snippet := citations[i].Snippet
		if len(snippet) > 180 {
			snippet = snippet[:180] + "..."
		}
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s [%s]", title, chunkID, snippet, citations[i].RefID))
	}
	lines = append(lines, "## Confidence")
	lines = append(lines, "- Medium confidence based on retrieved chunks; verify with full paper text.")
	return strings.Join(lines, "\n")
}

func (s *Server) handleSurvey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req struct {
		CorpusID  string   `json:"corpus_id"`
		Topics    []string `json:"topics"`
		Questions []string `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
		return
	}
	if strings.TrimSpace(req.CorpusID) == "" || len(req.Topics) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("corpus_id and at least one topic are required"))
		return
	}
	runID := uuid.NewString()
	if err := s.surveyRepo.CreateRun(r.Context(), runID, req.CorpusID, req.Topics, req.Questions); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	we, err := s.temporal.ExecuteWorkflow(r.Context(), tclient.StartWorkflowOptions{
		ID:        "survey-" + runID,
		TaskQueue: s.cfg.TemporalTaskQueue,
	}, workflows.SurveyBuildWorkflow, workflows.SurveyBuildInput{
		SurveyRunID:     runID,
		CorpusID:        req.CorpusID,
		Topics:          req.Topics,
		Questions:       req.Questions,
		EmbedProviders:  s.providers.EmbedCount(),
		LLMProviders:    s.providers.LLMCount(),
		CooldownSeconds: s.cfg.ProviderCooldownSecs,
	})
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"survey_run_id": runID, "workflow_id": we.GetID(), "run_id": we.GetRunID()})
}

func (s *Server) handleSurveyScoped(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/survey/"), "/"), "/")
	if len(parts) < 2 {
		writeErr(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	runID := parts[0]
	switch parts[1] {
	case "progress":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var prog workflows.SurveyProgress
		resp, err := s.temporal.QueryWorkflow(r.Context(), "survey-"+runID, "", workflows.QueryGetSurveyProgress)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if err := resp.Get(&prog); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, prog)
	case "report":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		outPath, status, err := s.surveyRepo.GetRunPath(r.Context(), runID)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		if outPath == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": status, "report_markdown": ""})
			return
		}
		b, err := os.ReadFile(outPath)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": status, "report_markdown": string(b), "path": outPath})
	default:
		writeErr(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func saveUploadedFile(dstDir string, fh *multipart.FileHeader) (paperID, path string, err error) {
	src, err := fh.Open()
	if err != nil {
		return "", "", fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp(dstDir, "upload-*.pdf")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), src); err != nil {
		return "", "", fmt.Errorf("write upload: %w", err)
	}

	paperID = fmt.Sprintf("%x", h.Sum(nil))
	safeName := filepath.Base(fh.Filename)
	finalPath := filepath.Join(dstDir, safeName)
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("seek temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmp.Name(), finalPath); err != nil {
		return "", "", fmt.Errorf("atomic move upload: %w", err)
	}

	return paperID, finalPath, nil
}

func firstSingleFile(m map[string][]*multipart.FileHeader) (*multipart.FileHeader, bool) {
	for _, v := range m {
		if len(v) > 0 {
			return v[0], true
		}
	}
	return nil, false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	apiErr := toAPIError(code, err)
	writeJSON(w, code, map[string]any{
		"error": map[string]any{
			"code":    apiErr.Code,
			"message": apiErr.Message,
		},
	})
}

type apiError struct {
	Code    string
	Message string
}

func toAPIError(status int, err error) apiError {
	msg := "Request failed."
	code := "LF-API-4000"
	raw := ""
	if err != nil {
		raw = strings.ToLower(err.Error())
	}

	switch {
	case status >= 500:
		switch {
		case strings.Contains(raw, "relation") && strings.Contains(raw, "does not exist"):
			return apiError{
				Code:    "LF-DB-5001",
				Message: "Database schema is not initialized. Run migrations and retry.",
			}
		case strings.Contains(raw, "connect"), strings.Contains(raw, "dial tcp"), strings.Contains(raw, "connection refused"):
			return apiError{
				Code:    "LF-DB-5002",
				Message: "Database connection is unavailable. Check local services and retry.",
			}
		default:
			return apiError{
				Code:    "LF-API-5000",
				Message: "Internal server error. Please retry or check service logs.",
			}
		}
	case status == http.StatusBadRequest:
		code = "LF-API-4001"
		msg = "Invalid request. Check inputs and retry."
	case status == http.StatusNotFound:
		code = "LF-API-4004"
		msg = "Requested resource was not found."
	case status == http.StatusConflict:
		code = "LF-API-4009"
		msg = "Operation conflicts with current state. Retry after checking status."
	case status == http.StatusMethodNotAllowed:
		code = "LF-API-4005"
		msg = "This endpoint does not support the requested method."
	case status == http.StatusBadGateway:
		code = "LF-API-5020"
		msg = "Upstream provider unavailable. Retry shortly."
	}

	// For 4xx, keep user-safe validation context only.
	if status >= 400 && status < 500 && err != nil {
		low := strings.ToLower(err.Error())
		switch {
		case strings.Contains(low, "name is required"):
			msg = "Corpus name is required."
		case strings.Contains(low, "corpus_id and question are required"):
			msg = "Both corpus and question are required."
		case strings.Contains(low, "no files provided"):
			msg = "No PDF files were provided."
		case strings.Contains(low, "invalid json"):
			msg = "Malformed JSON request body."
		}
	}

	return apiError{Code: code, Message: msg}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
