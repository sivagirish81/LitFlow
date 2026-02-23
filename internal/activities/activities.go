package activities

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"litflow/internal/config"
	"litflow/internal/models"
	"litflow/internal/providers"
	"litflow/internal/storage"
	"litflow/internal/util"
	"litflow/internal/vector"

	"github.com/ledongthuc/pdf"
)

type Activities struct {
	cfg          config.Config
	paperRepo    *storage.PaperRepo
	chunkRepo    *storage.ChunkRepo
	surveyRepo   *storage.SurveyRepo
	llmAuditRepo *storage.LLMAuditRepo
	graphRepo    *storage.GraphRepo
	searcher     *vector.Searcher
	providers    *providers.Manager
}

func New(cfg config.Config, db *storage.DB) (*Activities, error) {
	pm, err := providers.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	return &Activities{
		cfg:          cfg,
		paperRepo:    storage.NewPaperRepo(db),
		chunkRepo:    storage.NewChunkRepo(db),
		surveyRepo:   storage.NewSurveyRepo(db),
		llmAuditRepo: storage.NewLLMAuditRepo(db),
		graphRepo:    storage.NewGraphRepo(db),
		searcher:     vector.NewSearcher(db.Pool),
		providers:    pm,
	}, nil
}

func (a *Activities) ListPDFsActivity(ctx context.Context, in ListPDFsInput) (ListPDFsOutput, error) {
	_ = ctx
	entries, err := os.ReadDir(in.InputDir)
	if err != nil {
		return ListPDFsOutput{}, fmt.Errorf("read input dir: %w", err)
	}
	paths := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".pdf") {
			paths = append(paths, filepath.Join(in.InputDir, name))
		}
	}
	sort.Strings(paths)
	return ListPDFsOutput{Paths: paths}, nil
}

func (a *Activities) WriteCorpusSummaryActivity(ctx context.Context, in WriteCorpusSummaryInput) error {
	_ = ctx
	outPath := filepath.Join(a.cfg.DataOutRoot, in.CorpusID, "corpus_summary.json")
	return util.WriteJSONAtomic(outPath, in.Summary)
}

func (a *Activities) ListFailedPapersActivity(ctx context.Context, in ListFailedPapersInput) (ListFailedPapersOutput, error) {
	papers, err := a.paperRepo.ListFailedPapers(ctx, in.CorpusID)
	if err != nil {
		return ListFailedPapersOutput{}, err
	}
	out := ListFailedPapersOutput{Papers: make([]FailedPaper, 0, len(papers))}
	for _, p := range papers {
		out.Papers = append(out.Papers, FailedPaper{PaperID: p.PaperID, Filename: p.Filename})
	}
	return out, nil
}

func (a *Activities) ListCorpusPapersActivity(ctx context.Context, in ListCorpusPapersInput) (ListCorpusPapersOutput, error) {
	papers, err := a.paperRepo.ListPapersByCorpus(ctx, in.CorpusID)
	if err != nil {
		return ListCorpusPapersOutput{}, err
	}
	out := ListCorpusPapersOutput{Papers: make([]CorpusPaper, 0, len(papers))}
	for _, p := range papers {
		year := 0
		if p.Year != nil {
			year = *p.Year
		}
		out.Papers = append(out.Papers, CorpusPaper{
			PaperID:    p.PaperID,
			Filename:   p.Filename,
			Status:     p.Status,
			Title:      p.Title,
			Authors:    p.Authors,
			Year:       year,
			FailReason: p.FailReason,
		})
	}
	return out, nil
}

func (a *Activities) WriteRunManifestActivity(ctx context.Context, in WriteRunManifestInput) (WriteRunManifestOutput, error) {
	_ = ctx
	path := filepath.Join(a.cfg.DataOutRoot, in.CorpusID, "runs", in.RunID, "manifest.json")
	if err := util.WriteJSONAtomic(path, in.Manifest); err != nil {
		return WriteRunManifestOutput{}, err
	}
	return WriteRunManifestOutput{Path: path}, nil
}

func (a *Activities) ComputePaperIDActivity(ctx context.Context, in ComputePaperIDInput) (ComputePaperIDOutput, error) {
	_ = ctx
	f, err := os.Open(in.PaperPath)
	if err != nil {
		return ComputePaperIDOutput{}, fmt.Errorf("open file for hash: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ComputePaperIDOutput{}, fmt.Errorf("hash file: %w", err)
	}
	return ComputePaperIDOutput{PaperID: hex.EncodeToString(h.Sum(nil))}, nil
}

func (a *Activities) ExtractTextActivity(ctx context.Context, in ExtractTextInput) (ExtractTextOutput, error) {
	_ = ctx
	f, r, err := pdf.Open(in.PaperPath)
	if err != nil {
		return ExtractTextOutput{}, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return ExtractTextOutput{}, fmt.Errorf("extract pdf text: %w", err)
	}
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, reader); err != nil {
		return ExtractTextOutput{}, fmt.Errorf("read extracted text: %w", err)
	}
	text := strings.TrimSpace(buf.String())
	text = util.SanitizeText(text)
	if text == "" {
		return ExtractTextOutput{}, util.ErrNoExtractableText
	}
	return ExtractTextOutput{Text: text}, nil
}

func (a *Activities) ExtractMetadataActivity(ctx context.Context, in ExtractMetadataInput) (ExtractMetadataOutput, error) {
	_ = ctx
	title, authors := heuristicTitleAndAuthors(in.Text)
	return ExtractMetadataOutput{Title: title, Authors: authors}, nil
}

func (a *Activities) ChunkTextActivity(ctx context.Context, in ChunkTextInput) (ChunkTextOutput, error) {
	_ = ctx
	if in.ChunkSize <= 0 {
		in.ChunkSize = a.cfg.ChunkSize
	}
	if in.ChunkOverlap < 0 || in.ChunkOverlap >= in.ChunkSize {
		in.ChunkOverlap = a.cfg.ChunkOverlap
	}

	rawChunks := util.ChunkText(in.Text, in.ChunkSize, in.ChunkOverlap)
	chunks := make([]ChunkItem, 0, len(rawChunks))
	for idx, part := range rawChunks {
		part = util.SanitizeText(part)
		if part == "" {
			continue
		}
		chunkHash := util.SHA256Hex([]byte(part))
		chunkID := util.SHA256Hex([]byte(fmt.Sprintf("%s:%d:%s:%s", in.PaperID, idx, chunkHash, in.Version)))
		chunks = append(chunks, ChunkItem{
			ChunkID:    chunkID,
			PaperID:    in.PaperID,
			CorpusID:   in.CorpusID,
			ChunkIndex: idx,
			Text:       part,
		})
	}
	return ChunkTextOutput{Chunks: chunks}, nil
}

func (a *Activities) UpsertChunksActivity(ctx context.Context, in UpsertChunksInput) error {
	records := make([]storage.ChunkRecord, 0, len(in.Chunks))
	for i, c := range in.Chunks {
		var embedding *string
		if i < len(in.Vectors) && len(in.Vectors[i]) > 0 {
			lit := vector.ToLiteral(in.Vectors[i])
			embedding = &lit
		}
		records = append(records, storage.ChunkRecord{
			ChunkID:          c.ChunkID,
			PaperID:          c.PaperID,
			CorpusID:         c.CorpusID,
			ChunkIndex:       c.ChunkIndex,
			Text:             util.SanitizeText(c.Text),
			EmbeddingVersion: in.EmbeddingVersion,
			EmbeddingVector:  embedding,
		})
	}
	return a.chunkRepo.UpsertChunks(ctx, records)
}

func (a *Activities) WritePaperArtifactsActivity(ctx context.Context, in WritePaperArtifactsInput) error {
	_ = ctx
	base := filepath.Join(a.cfg.DataOutRoot, in.CorpusID, "papers", in.PaperID)
	if err := util.EnsureDir(base); err != nil {
		return err
	}
	if err := util.WriteJSONAtomic(filepath.Join(base, "metadata.json"), in.Metadata); err != nil {
		return err
	}
	rows := make([]any, 0, len(in.Chunks))
	for _, c := range in.Chunks {
		rows = append(rows, c)
	}
	if err := util.WriteJSONLinesAtomic(filepath.Join(base, "chunks.jsonl"), rows); err != nil {
		return err
	}
	if err := util.WriteJSONAtomic(filepath.Join(base, "processing_log.json"), in.ProcessingLog); err != nil {
		return err
	}
	return nil
}

func (a *Activities) UpdatePaperStatusActivity(ctx context.Context, in UpdatePaperStatusInput) error {
	return a.paperRepo.UpsertPaper(ctx, models.Paper{
		PaperID:    in.PaperID,
		CorpusID:   in.CorpusID,
		Filename:   in.Filename,
		Title:      in.Title,
		Authors:    in.Authors,
		Status:     in.Status,
		FailReason: in.FailReason,
	})
}

func (a *Activities) EmbedChunksActivity(ctx context.Context, in EmbedChunksInput) (EmbedChunksOutput, error) {
	inputs := make([]string, 0, len(in.Input))
	for _, c := range in.Input {
		inputs = append(inputs, c.Text)
	}
	provider, _ := a.providers.EmbedProviderByIndex(in.ProviderIndex)
	vectors, info, err := provider.Embed(ctx, providers.EmbedRequest{
		Operation: in.Operation,
		Inputs:    inputs,
		Dimension: a.cfg.EmbedDim,
	})
	if err != nil {
		return EmbedChunksOutput{}, err
	}
	return EmbedChunksOutput{
		Vectors:      vectors,
		ProviderName: info.Name,
		Model:        info.Model,
	}, nil
}

func (a *Activities) LLMGenerateActivity(ctx context.Context, in LLMGenerateInput) (LLMGenerateOutput, error) {
	if in.ProviderRef != "" {
		if idx := a.providers.FindLLMProviderIndex(in.ProviderRef); idx >= 0 {
			in.ProviderIndex = idx
		} else {
			return LLMGenerateOutput{}, fmt.Errorf("llm provider ref not configured in worker: %s", in.ProviderRef)
		}
	}
	provider, ref := a.providers.LLMProviderByIndex(in.ProviderIndex)
	resp, info, err := provider.Generate(ctx, providers.GenerateRequest{
		Operation: in.Operation,
		Prompt:    in.Prompt,
		Context:   in.Context,
	})
	if err != nil {
		return LLMGenerateOutput{}, fmt.Errorf("llm generate via %s failed: %w", ref.Raw, err)
	}
	return LLMGenerateOutput{
		Text:         resp.Text,
		ProviderName: info.Name,
		Model:        info.Model,
	}, nil
}

func (a *Activities) EmbedQueryActivity(ctx context.Context, in EmbedQueryInput) (EmbedQueryOutput, error) {
	provider, _ := a.providers.EmbedProviderByIndex(in.ProviderIndex)
	vectors, info, err := provider.Embed(ctx, providers.EmbedRequest{
		Operation: in.Operation,
		Inputs:    []string{in.Text},
		Dimension: a.cfg.EmbedDim,
	})
	if err != nil {
		return EmbedQueryOutput{}, err
	}
	if len(vectors) == 0 {
		return EmbedQueryOutput{}, fmt.Errorf("embedding provider returned empty vectors")
	}
	return EmbedQueryOutput{Vector: vectors[0], ProviderName: info.Name, Model: info.Model}, nil
}

func (a *Activities) SearchChunksActivity(ctx context.Context, in SearchChunksInput) (SearchChunksOutput, error) {
	results, err := a.searcher.SearchChunks(ctx, in.CorpusID, in.QueryVec, in.TopK, vector.SearchFilters{
		EmbeddingVersion: in.EmbeddingVersion,
	})
	if err != nil {
		return SearchChunksOutput{}, err
	}
	out := make([]SearchChunk, 0, len(results))
	for _, r := range results {
		out = append(out, SearchChunk{
			PaperID: r.PaperID,
			Title:   r.Title,
			ChunkID: r.ChunkID,
			Snippet: r.Snippet,
			Score:   r.Score,
			Text:    r.ChunkText,
		})
	}
	return SearchChunksOutput{Results: out}, nil
}

func (a *Activities) WriteSurveyReportActivity(ctx context.Context, in WriteSurveyReportInput) (WriteSurveyReportOutput, error) {
	_ = ctx
	ext := "md"
	if strings.EqualFold(strings.TrimSpace(in.OutputFormat), "latex") {
		ext = "tex"
	}
	outPath := filepath.Join(a.cfg.DataOutRoot, in.CorpusID, "surveys", in.SurveyRunID, "report."+ext)
	if err := util.WriteTextAtomic(outPath, in.Report); err != nil {
		return WriteSurveyReportOutput{}, err
	}
	return WriteSurveyReportOutput{OutPath: outPath}, nil
}

func (a *Activities) UpdateSurveyRunActivity(ctx context.Context, in UpdateSurveyRunInput) error {
	return a.surveyRepo.UpdateRunStatus(ctx, in.SurveyRunID, in.Status, in.OutPath)
}

func (a *Activities) LogLLMCallActivity(ctx context.Context, in LogLLMCallInput) error {
	return a.llmAuditRepo.Insert(ctx, storage.LLMCallRecord{
		CallID:       in.CallID,
		Operation:    in.Operation,
		CorpusID:     in.CorpusID,
		PaperID:      in.PaperID,
		ProviderName: in.ProviderName,
		Model:        in.Model,
		RequestID:    in.RequestID,
		Status:       in.Status,
		ErrorType:    in.ErrorType,
	})
}

func (a *Activities) UpsertTopicGraphActivity(ctx context.Context, in UpsertTopicGraphInput) error {
	return a.graphRepo.UpsertTopicRetrieval(ctx, in.CorpusID, in.Topic, in.PaperID, in.Title, in.Score, in.ChunkID)
}

func (a *Activities) GetSurveyPaperMetaActivity(ctx context.Context, in GetSurveyPaperMetaInput) (GetSurveyPaperMetaOutput, error) {
	papers, err := a.paperRepo.ListPapersByIDs(ctx, in.CorpusID, in.PaperIDs)
	if err != nil {
		return GetSurveyPaperMetaOutput{}, err
	}
	out := GetSurveyPaperMetaOutput{Papers: make([]SurveyPaperMeta, 0, len(papers))}
	for _, p := range papers {
		year := 0
		if p.Year != nil {
			year = *p.Year
		}
		out.Papers = append(out.Papers, SurveyPaperMeta{
			PaperID:  p.PaperID,
			Title:    p.Title,
			Authors:  p.Authors,
			Year:     year,
			Filename: p.Filename,
		})
	}
	return out, nil
}

func (a *Activities) ListPaperChunksActivity(ctx context.Context, in KGPaperInput) (ListPaperChunksOutput, error) {
	paper, err := a.paperRepo.GetPaperByID(ctx, in.CorpusID, in.PaperID)
	if err != nil {
		return ListPaperChunksOutput{}, err
	}
	chunks, err := a.chunkRepo.ListChunksByPaper(ctx, in.CorpusID, in.PaperID)
	if err != nil {
		return ListPaperChunksOutput{}, err
	}
	out := ListPaperChunksOutput{
		Title:  paper.Title,
		Chunks: make([]KGPaperChunk, 0, len(chunks)),
	}
	for _, c := range chunks {
		out.Chunks = append(out.Chunks, KGPaperChunk{ChunkID: c.ChunkID, Text: c.Text})
	}
	return out, nil
}

func (a *Activities) UpsertKGTriplesActivity(ctx context.Context, in UpsertKGTriplesInput) error {
	triples := make([]storage.KGTripleInput, 0, len(in.Triples))
	for _, t := range in.Triples {
		triples = append(triples, storage.KGTripleInput{
			CorpusID:     in.CorpusID,
			PaperID:      in.PaperID,
			PromptHash:   in.PromptHash,
			ModelVersion: in.ModelVersion,
			SourceType:   t.SourceType,
			SourceName:   t.SourceName,
			RelationType: t.RelationType,
			TargetType:   t.TargetType,
			TargetName:   t.TargetName,
			ChunkID:      t.ChunkID,
			Evidence:     t.Evidence,
			Confidence:   t.Confidence,
		})
	}
	return a.graphRepo.UpsertKGTriples(ctx, triples)
}

func (a *Activities) MarkKGPaperRunActivity(ctx context.Context, in MarkKGPaperRunInput) error {
	return a.graphRepo.UpsertKGRun(ctx, storage.KGRunRecord{
		CorpusID:     in.CorpusID,
		PaperID:      in.PaperID,
		PromptHash:   in.PromptHash,
		ModelVersion: in.ModelVersion,
		Status:       in.Status,
		TripleCount:  in.TripleCount,
		LastError:    in.LastError,
	})
}

func heuristicTitleAndAuthors(text string) (string, string) {
	s := bufio.NewScanner(strings.NewReader(text))
	nonEmpty := make([]string, 0, 4)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		nonEmpty = append(nonEmpty, line)
		if len(nonEmpty) == 4 {
			break
		}
	}
	title := ""
	authors := ""
	if len(nonEmpty) > 0 {
		title = nonEmpty[0]
	}
	if len(nonEmpty) > 1 {
		authors = nonEmpty[1]
	}
	return title, authors
}
