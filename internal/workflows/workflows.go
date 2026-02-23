package workflows

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"litflow/internal/activities"
	"litflow/internal/providers"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	QueryGetPaperStatus    = "GetPaperStatus"
	QueryGetProgress       = "GetProgress"
	QueryGetSurveyProgress = "GetSurveyProgress"
)

type providerState struct {
	disabledUntil map[int]time.Time
	retries       map[string]int
}

func newProviderState() providerState {
	return providerState{disabledUntil: map[int]time.Time{}, retries: map[string]int{}}
}

func CorpusIngestWorkflow(ctx workflow.Context, input CorpusIngestInput) (string, error) {
	progress := CorpusIngestProgress{
		CorpusID:      input.CorpusID,
		PerPaper:      map[string]string{},
		ChildWorkflow: map[string]string{},
	}
	if err := workflow.SetQueryHandler(ctx, QueryGetProgress, func() (CorpusIngestProgress, error) {
		return progress, nil
	}); err != nil {
		return "", err
	}

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    20 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	var listOut activities.ListPDFsOutput
	if err := workflow.ExecuteActivity(ctx, "ListPDFsActivity", activities.ListPDFsInput{InputDir: input.InputDir}).Get(ctx, &listOut); err != nil {
		return "", err
	}
	paths := listOut.Paths
	progress.Total = len(paths)
	maxChildren := input.MaxConcurrentChildren
	if maxChildren <= 0 {
		maxChildren = 3
	}

	for i := 0; i < len(paths); i += maxChildren {
		end := i + maxChildren
		if end > len(paths) {
			end = len(paths)
		}
		futures := make([]workflow.ChildWorkflowFuture, 0, end-i)
		childPaths := make([]string, 0, end-i)
		for _, path := range paths[i:end] {
			progress.PerPaper[path] = "processing"
			workflowID := "paper-" + sanitizeID(input.CorpusID) + "-" + sanitizeID(filepathBase(path))
			cwo := workflow.ChildWorkflowOptions{WorkflowID: workflowID}
			childCtx := workflow.WithChildOptions(ctx, cwo)
			f := workflow.ExecuteChildWorkflow(childCtx, PaperProcessWorkflow, PaperProcessInput{
				CorpusID:        input.CorpusID,
				PaperPath:       path,
				ChunkVersion:    defaultChunkVersion(input.ChunkVersion),
				EmbedVersion:    defaultEmbedVersion(input.EmbedVersion),
				EmbedProviders:  input.EmbedProviders,
				CooldownSeconds: input.CooldownSeconds,
			})
			futures = append(futures, f)
			childPaths = append(childPaths, path)
			progress.ChildWorkflow[path] = workflowID
		}

		for idx, f := range futures {
			var childStatus string
			err := f.Get(ctx, &childStatus)
			path := childPaths[idx]
			if err != nil {
				progress.Failed++
				progress.PerPaper[path] = "failed"
				continue
			}
			if childStatus == "failed" {
				progress.Failed++
			}
			progress.Done++
			progress.PerPaper[path] = childStatus
		}
	}
	_ = workflow.ExecuteActivity(ctx, "WriteCorpusSummaryActivity", activities.WriteCorpusSummaryInput{
		CorpusID: input.CorpusID,
		Summary: map[string]any{
			"corpus_id":        input.CorpusID,
			"total":            progress.Total,
			"done":             progress.Done,
			"failed":           progress.Failed,
			"per_paper_status": progress.PerPaper,
			"generated_at":     workflow.Now(ctx),
		},
	}).Get(ctx, nil)

	return "completed", nil
}

func PaperProcessWorkflow(ctx workflow.Context, input PaperProcessInput) (string, error) {
	status := PaperStatus{
		PaperPath:   input.PaperPath,
		CurrentStep: "init",
		Status:      "processing",
		RetryCounts: map[string]int{},
		Steps:       map[string]string{},
	}
	if err := workflow.SetQueryHandler(ctx, QueryGetPaperStatus, func() (PaperStatus, error) {
		return status, nil
	}); err != nil {
		return "", err
	}

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    20 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	filename := filepath.Base(input.PaperPath)
	cooldown := durationOrDefault(input.CooldownSeconds, 900)
	providerCount := input.EmbedProviders
	if providerCount <= 0 {
		providerCount = 1
	}
	state := newProviderState()

	status.CurrentStep = "compute_paper_id"
	status.Steps[status.CurrentStep] = "processing"
	var computeOut activities.ComputePaperIDOutput
	if err := workflow.ExecuteActivity(ctx, "ComputePaperIDActivity", activities.ComputePaperIDInput{PaperPath: input.PaperPath}).Get(ctx, &computeOut); err != nil {
		return "", err
	}
	status.PaperID = computeOut.PaperID
	status.Steps[status.CurrentStep] = "done"

	_ = workflow.ExecuteActivity(ctx, "UpdatePaperStatusActivity", activities.UpdatePaperStatusInput{PaperID: computeOut.PaperID, CorpusID: input.CorpusID, Filename: filename, Status: "processing"})

	status.CurrentStep = "extract_text"
	status.Steps[status.CurrentStep] = "processing"
	var textOut activities.ExtractTextOutput
	if err := workflow.ExecuteActivity(ctx, "ExtractTextActivity", activities.ExtractTextInput{PaperPath: input.PaperPath}).Get(ctx, &textOut); err != nil {
		if isNoTextError(err) {
			status.Status = "failed"
			status.FailReason = "no extractable text found (OCR not enabled)"
			status.Steps[status.CurrentStep] = "failed"
			_ = workflow.ExecuteActivity(ctx, "UpdatePaperStatusActivity", activities.UpdatePaperStatusInput{PaperID: computeOut.PaperID, CorpusID: input.CorpusID, Filename: filename, Status: "failed", FailReason: status.FailReason})
			return status.Status, nil
		}
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "extract_metadata"
	status.Steps[status.CurrentStep] = "processing"
	var metaOut activities.ExtractMetadataOutput
	if err := workflow.ExecuteActivity(ctx, "ExtractMetadataActivity", activities.ExtractMetadataInput(textOut)).Get(ctx, &metaOut); err != nil {
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "chunk_text"
	status.Steps[status.CurrentStep] = "processing"
	var chunkOut activities.ChunkTextOutput
	if err := workflow.ExecuteActivity(ctx, "ChunkTextActivity", activities.ChunkTextInput{PaperID: computeOut.PaperID, CorpusID: input.CorpusID, Text: textOut.Text, ChunkSize: input.ChunkSize, ChunkOverlap: input.ChunkOverlap, Version: defaultChunkVersion(input.ChunkVersion)}).Get(ctx, &chunkOut); err != nil {
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "embed_chunks"
	status.Steps[status.CurrentStep] = "processing"
	var embedOut activities.EmbedChunksOutput
	var err error
	if input.PreferredEmbedProviderIndex >= 0 {
		embedOut, err = callEmbedWithFailover(ctx, &state, providerCount, cooldown, activities.EmbedChunksInput{
			Operation: "embed",
			CorpusID:  input.CorpusID,
			PaperID:   computeOut.PaperID,
			Input:     chunkOut.Chunks,
		}, status.RetryCounts, input.PreferredEmbedProviderIndex, input.StrictEmbedProvider)
	} else {
		embedOut, err = callEmbedWithFailover(ctx, &state, providerCount, cooldown, activities.EmbedChunksInput{
			Operation: "embed",
			CorpusID:  input.CorpusID,
			PaperID:   computeOut.PaperID,
			Input:     chunkOut.Chunks,
		}, status.RetryCounts, -1, false)
	}
	if err != nil {
		return "", err
	}
	status.Providers = append(status.Providers, embedOut.ProviderName)
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "upsert_chunks"
	status.Steps[status.CurrentStep] = "processing"
	if err := workflow.ExecuteActivity(ctx, "UpsertChunksActivity", activities.UpsertChunksInput{Chunks: chunkOut.Chunks, Vectors: embedOut.Vectors, EmbeddingVersion: defaultEmbedVersion(input.EmbedVersion)}).Get(ctx, nil); err != nil {
		if isInvalidTextEncodingError(err) {
			status.Status = "failed"
			status.FailReason = "paper contains invalid text encoding after extraction"
			status.Steps[status.CurrentStep] = "failed"
			_ = workflow.ExecuteActivity(ctx, "UpdatePaperStatusActivity", activities.UpdatePaperStatusInput{
				PaperID:    computeOut.PaperID,
				CorpusID:   input.CorpusID,
				Filename:   filename,
				Status:     "failed",
				FailReason: status.FailReason,
			}).Get(ctx, nil)
			return status.Status, nil
		}
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "write_artifacts"
	status.Steps[status.CurrentStep] = "processing"
	if err := workflow.ExecuteActivity(ctx, "WritePaperArtifactsActivity", activities.WritePaperArtifactsInput{CorpusID: input.CorpusID, PaperID: computeOut.PaperID, Metadata: map[string]any{"paper_id": computeOut.PaperID, "filename": filename, "title": metaOut.Title, "authors": metaOut.Authors, "chunk_count": len(chunkOut.Chunks)}, Chunks: chunkOut.Chunks, ProcessingLog: map[string]any{"status": "processed", "steps": status.Steps, "generated_at": workflow.Now(ctx)}}).Get(ctx, nil); err != nil {
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"

	status.CurrentStep = "mark_processed"
	status.Steps[status.CurrentStep] = "processing"
	if err := workflow.ExecuteActivity(ctx, "UpdatePaperStatusActivity", activities.UpdatePaperStatusInput{PaperID: computeOut.PaperID, CorpusID: input.CorpusID, Filename: filename, Title: metaOut.Title, Authors: metaOut.Authors, Status: "processed"}).Get(ctx, nil); err != nil {
		return "", err
	}
	status.Steps[status.CurrentStep] = "done"
	status.CurrentStep = "done"
	status.Status = "processed"
	return status.Status, nil
}

func SurveyBuildWorkflow(ctx workflow.Context, input SurveyBuildInput) (string, error) {
	topic := strings.TrimSpace(input.Prompt)
	if topic == "" && len(input.Topics) > 0 {
		topic = strings.TrimSpace(input.Topics[0])
	}
	if topic == "" {
		return "", fmt.Errorf("survey prompt/topic is required")
	}
	topicLabel := topic
	if len(topicLabel) > 64 {
		topicLabel = topicLabel[:61] + "..."
	}
	progress := SurveyProgress{SurveyRunID: input.SurveyRunID, CorpusID: input.CorpusID, TotalTopics: 1, TopicStatus: map[string]string{}}
	if err := workflow.SetQueryHandler(ctx, QueryGetSurveyProgress, func() (SurveyProgress, error) { return progress, nil }); err != nil {
		return "", err
	}
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	_ = workflow.ExecuteActivity(ctx, "UpdateSurveyRunActivity", activities.UpdateSurveyRunInput{SurveyRunID: input.SurveyRunID, Status: "running"}).Get(ctx, nil)

	embedProviders := input.EmbedProviders
	if embedProviders <= 0 {
		embedProviders = 1
	}
	llmProviders := input.LLMProviders
	if llmProviders <= 0 {
		llmProviders = 1
	}
	cooldown := durationOrDefault(input.CooldownSeconds, 900)
	embedState := newProviderState()
	llmState := newProviderState()
	topK := input.RetrievalTopK
	if topK <= 0 {
		topK = 14
	}

	progress.TopicStatus[topicLabel] = "retrieving"
	eq, err := callEmbedQueryWithFailover(ctx, &embedState, embedProviders, cooldown, activities.EmbedQueryInput{
		Operation: "survey_topic_embed",
		Text:      topic,
	}, nil)
	if err != nil {
		progress.TopicStatus[topicLabel] = "failed"
		return "", err
	}
	var retrieved activities.SearchChunksOutput
	if err := workflow.ExecuteActivity(ctx, "SearchChunksActivity", activities.SearchChunksInput{
		CorpusID:         input.CorpusID,
		QueryVec:         eq.Vector,
		TopK:             topK,
		EmbeddingVersion: defaultEmbedVersion(input.EmbedVersion),
	}).Get(ctx, &retrieved); err != nil {
		progress.TopicStatus[topicLabel] = "failed"
		return "", err
	}
	for _, c := range retrieved.Results {
		_ = workflow.ExecuteActivity(ctx, "UpsertTopicGraphActivity", activities.UpsertTopicGraphInput{
			CorpusID: input.CorpusID,
			Topic:    topic,
			PaperID:  c.PaperID,
			Title:    c.Title,
			ChunkID:  c.ChunkID,
			Score:    c.Score,
		}).Get(ctx, nil)
	}
	progress.TopicStatus[topicLabel] = "drafting"

	refs, contextWindow := buildSurveyReferences(retrieved.Results)
	paperIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.PaperID) != "" {
			paperIDs = append(paperIDs, ref.PaperID)
		}
	}
	if len(paperIDs) > 0 {
		var metaOut activities.GetSurveyPaperMetaOutput
		if err := workflow.ExecuteActivity(ctx, "GetSurveyPaperMetaActivity", activities.GetSurveyPaperMetaInput{
			CorpusID: input.CorpusID,
			PaperIDs: paperIDs,
		}).Get(ctx, &metaOut); err == nil {
			metaByID := make(map[string]activities.SurveyPaperMeta, len(metaOut.Papers))
			for _, m := range metaOut.Papers {
				metaByID[m.PaperID] = m
			}
			for i := range refs {
				m, ok := metaByID[refs[i].PaperID]
				if !ok {
					continue
				}
				if strings.TrimSpace(m.Title) != "" {
					refs[i].Title = strings.TrimSpace(m.Title)
				}
				refs[i].Authors = strings.TrimSpace(m.Authors)
				refs[i].Year = m.Year
				refs[i].Filename = strings.TrimSpace(m.Filename)
			}
		}
	}
	sectionInput := activities.LLMGenerateInput{
		Operation: "survey_ieee_latex",
		CorpusID:  input.CorpusID,
		Prompt:    buildLatexPrompt(topic, refs),
		Context:   contextWindow,
	}
	section, sectionErrType, sectionErr := callLLMWithFailover(ctx, &llmState, llmProviders, input.LLMProviderRefs, cooldown, sectionInput, nil)
	if sectionErr != nil && sectionErrType == string(providers.ErrorContext) {
		reduced := contextWindow
		if len(reduced) > 5 {
			reduced = reduced[:5]
		}
		sectionInput.Context = reduced
		section, _, sectionErr = callLLMWithFailover(ctx, &llmState, llmProviders, input.LLMProviderRefs, cooldown, sectionInput, nil)
	}

	report := buildLatexDocument(topic, refs, cleanLLMDocument(section.Text), sectionErr != nil)
	if strings.TrimSpace(input.OutputFormat) == "" {
		input.OutputFormat = "latex"
	}

	var reportOut activities.WriteSurveyReportOutput
	if err := workflow.ExecuteActivity(ctx, "WriteSurveyReportActivity", activities.WriteSurveyReportInput{
		CorpusID:     input.CorpusID,
		SurveyRunID:  input.SurveyRunID,
		Report:       report,
		OutputFormat: input.OutputFormat,
	}).Get(ctx, &reportOut); err != nil {
		return "", err
	}
	progress.TopicStatus[topicLabel] = "done"
	progress.DoneTopics = 1
	_ = workflow.ExecuteActivity(ctx, "UpdateSurveyRunActivity", activities.UpdateSurveyRunInput{SurveyRunID: input.SurveyRunID, Status: "completed", OutPath: reportOut.OutPath}).Get(ctx, nil)
	return reportOut.OutPath, nil
}

func BackfillWorkflow(ctx workflow.Context, input BackfillInput) (string, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	info := workflow.GetInfo(ctx)
	runID := info.WorkflowExecution.RunID
	manifest := map[string]any{
		"run_id":     runID,
		"mode":       input.Mode,
		"corpus_id":  input.CorpusID,
		"versions":   map[string]any{"chunk": defaultChunkVersion(input.ChunkVersion), "embed": defaultEmbedVersion(input.EmbedVersion), "survey_prompt": "v1"},
		"started_at": workflow.Now(ctx),
	}

	switch strings.ToUpper(strings.TrimSpace(input.Mode)) {
	case "RETRY_FAILED_PAPERS":
		var failed activities.ListFailedPapersOutput
		if err := workflow.ExecuteActivity(ctx, "ListFailedPapersActivity", activities.ListFailedPapersInput{CorpusID: input.CorpusID}).Get(ctx, &failed); err != nil {
			return "", err
		}
		retried := 0
		for _, p := range failed.Papers {
			path := pathForBackfill(input, p.Filename)
			var out string
			if err := workflow.ExecuteChildWorkflow(ctx, PaperProcessWorkflow, PaperProcessInput{
				CorpusID:                    input.CorpusID,
				PaperPath:                   path,
				ChunkVersion:                defaultChunkVersion(input.ChunkVersion),
				EmbedVersion:                defaultEmbedVersion(input.EmbedVersion),
				EmbedProviders:              defaultCount(input.EmbedProviders),
				PreferredEmbedProviderIndex: input.PreferredEmbedProviderIndex,
				StrictEmbedProvider:         input.StrictEmbedProvider,
				CooldownSeconds:             defaultSeconds(input.CooldownSeconds, 900),
			}).Get(ctx, &out); err == nil {
				retried++
			}
		}
		manifest["retried_failed_papers"] = retried
	case "REEMBED_ALL_PAPERS":
		var all activities.ListCorpusPapersOutput
		if err := workflow.ExecuteActivity(ctx, "ListCorpusPapersActivity", activities.ListCorpusPapersInput{CorpusID: input.CorpusID}).Get(ctx, &all); err != nil {
			return "", err
		}
		processed := 0
		for _, p := range all.Papers {
			if strings.TrimSpace(p.Filename) == "" {
				continue
			}
			path := pathForBackfill(input, p.Filename)
			var out string
			if err := workflow.ExecuteChildWorkflow(ctx, PaperProcessWorkflow, PaperProcessInput{
				CorpusID:                    input.CorpusID,
				PaperPath:                   path,
				ChunkVersion:                defaultChunkVersion(input.ChunkVersion),
				EmbedVersion:                defaultEmbedVersion(input.EmbedVersion),
				EmbedProviders:              defaultCount(input.EmbedProviders),
				PreferredEmbedProviderIndex: input.PreferredEmbedProviderIndex,
				StrictEmbedProvider:         input.StrictEmbedProvider,
				CooldownSeconds:             defaultSeconds(input.CooldownSeconds, 900),
			}).Get(ctx, &out); err == nil {
				processed++
			}
		}
		manifest["reembedded_papers"] = processed
		manifest["total_papers_seen"] = len(all.Papers)
	case "REGENERATE_SURVEY":
		run := input.SurveyRunID
		if strings.TrimSpace(run) == "" {
			run = sanitizeID(fmt.Sprintf("%s-%d", input.CorpusID, workflow.Now(ctx).Unix()))
		}
		var outPath string
		if err := workflow.ExecuteChildWorkflow(ctx, SurveyBuildWorkflow, SurveyBuildInput{
			SurveyRunID:     run,
			CorpusID:        input.CorpusID,
			Topics:          input.Topics,
			Questions:       input.Questions,
			EmbedProviders:  defaultCount(input.EmbedProviders),
			LLMProviders:    defaultCount(input.LLMProviders),
			LLMProviderRefs: input.LLMProviderRefs,
			CooldownSeconds: defaultSeconds(input.CooldownSeconds, 900),
			EmbedVersion:    defaultEmbedVersion(input.EmbedVersion),
		}).Get(ctx, &outPath); err != nil {
			return "", err
		}
		manifest["regenerated_survey_run_id"] = run
		manifest["report_path"] = outPath
	default:
		return "", fmt.Errorf("unsupported backfill mode: %s", input.Mode)
	}

	var out activities.WriteRunManifestOutput
	if err := workflow.ExecuteActivity(ctx, "WriteRunManifestActivity", activities.WriteRunManifestInput{
		CorpusID: input.CorpusID,
		RunID:    runID,
		Manifest: manifest,
	}).Get(ctx, &out); err != nil {
		return "", err
	}
	return out.Path, nil
}

func callEmbedWithFailover(ctx workflow.Context, state *providerState, providerCount int, cooldown time.Duration, input activities.EmbedChunksInput, retryCounts map[string]int, preferredIdx int, strict bool) (activities.EmbedChunksOutput, error) {
	if retryCounts == nil {
		retryCounts = map[string]int{}
	}
	var lastErr error
	maxAttempts := providerCount * 4
	if maxAttempts <= 0 {
		maxAttempts = 4
	}
	if strict && preferredIdx >= 0 {
		maxAttempts = 4
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		idx := 0
		if strict && preferredIdx >= 0 {
			idx = preferredIdx
		} else if preferredIdx >= 0 {
			idx = (preferredIdx + attempt) % providerCount
		} else {
			idx = attempt % providerCount
		}
		if isProviderDisabled(ctx, state, idx) {
			continue
		}
		input.ProviderIndex = idx
		var out activities.EmbedChunksOutput
		err := workflow.ExecuteActivity(ctx, "EmbedChunksActivity", input).Get(ctx, &out)
		if err == nil {
			_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: out.ProviderName, Model: out.Model, RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "ok"}).Get(ctx, nil)
			return out, nil
		}
		lastErr = err
		errType := providers.ClassifyError(err)
		_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: fmt.Sprintf("provider-%d", idx), RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "failed", ErrorType: string(errType)}).Get(ctx, nil)
		key := fmt.Sprintf("embed-%d", idx)
		retryCounts[key]++
		switch errType {
		case providers.ErrorQuota:
			disableProviderUntil(ctx, state, idx, cooldown)
		case providers.ErrorRate:
			if retryCounts[key] <= 2 {
				if err := workflow.Sleep(ctx, time.Duration(retryCounts[key]*2)*time.Second); err != nil {
					return activities.EmbedChunksOutput{}, err
				}
				if !strict {
					attempt--
				}
			} else {
				disableProviderUntil(ctx, state, idx, 2*time.Minute)
			}
		case providers.ErrorTransient:
			if retryCounts[key] <= 2 {
				if err := workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second); err != nil {
					return activities.EmbedChunksOutput{}, err
				}
				if !strict {
					attempt--
				}
			}
		default:
			disableProviderUntil(ctx, state, idx, time.Minute)
		}
		if strict {
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all embed providers exhausted")
	}
	return activities.EmbedChunksOutput{}, lastErr
}

func callEmbedQueryWithFailover(ctx workflow.Context, state *providerState, providerCount int, cooldown time.Duration, input activities.EmbedQueryInput, retryCounts map[string]int) (activities.EmbedQueryOutput, error) {
	if retryCounts == nil {
		retryCounts = map[string]int{}
	}
	var lastErr error
	for attempt := 0; attempt < providerCount*4; attempt++ {
		idx := attempt % providerCount
		if isProviderDisabled(ctx, state, idx) {
			continue
		}
		input.ProviderIndex = idx
		var out activities.EmbedQueryOutput
		err := workflow.ExecuteActivity(ctx, "EmbedQueryActivity", input).Get(ctx, &out)
		if err == nil {
			return out, nil
		}
		lastErr = err
		errType := providers.ClassifyError(err)
		key := fmt.Sprintf("eq-%d", idx)
		retryCounts[key]++
		switch errType {
		case providers.ErrorQuota:
			disableProviderUntil(ctx, state, idx, cooldown)
		case providers.ErrorRate, providers.ErrorTransient:
			if retryCounts[key] <= 2 {
				if err := workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second); err != nil {
					return activities.EmbedQueryOutput{}, err
				}
				attempt--
			} else {
				disableProviderUntil(ctx, state, idx, 2*time.Minute)
			}
		default:
			disableProviderUntil(ctx, state, idx, time.Minute)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all embed query providers exhausted")
	}
	return activities.EmbedQueryOutput{}, lastErr
}

func callLLMWithFailover(ctx workflow.Context, state *providerState, providerCount int, providerRefs []string, cooldown time.Duration, input activities.LLMGenerateInput, retryCounts map[string]int) (activities.LLMGenerateOutput, string, error) {
	if retryCounts == nil {
		retryCounts = map[string]int{}
	}
	if len(providerRefs) > 0 {
		providerCount = len(providerRefs)
	}
	var lastErr error
	for attempt := 0; attempt < providerCount*4; attempt++ {
		idx := attempt % providerCount
		if isProviderDisabled(ctx, state, idx) {
			continue
		}
		selectedRef := ""
		if idx < len(providerRefs) {
			selectedRef = providerRefs[idx]
		}
		input.ProviderRef = selectedRef
		input.ProviderIndex = idx
		var out activities.LLMGenerateOutput
		err := workflow.ExecuteActivity(ctx, "LLMGenerateActivity", input).Get(ctx, &out)
		if err == nil {
			_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: out.ProviderName, Model: out.Model, RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "ok"}).Get(ctx, nil)
			return out, "", nil
		}
		lastErr = err
		errType := providers.ClassifyError(err)
		providerName := fmt.Sprintf("provider-%d", idx)
		if selectedRef != "" {
			providerName = selectedRef
		}
		_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: providerName, RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "failed", ErrorType: string(errType)}).Get(ctx, nil)
		key := fmt.Sprintf("llm-%s-%d", input.Operation, idx)
		retryCounts[key]++
		switch errType {
		case providers.ErrorQuota:
			disableProviderUntil(ctx, state, idx, cooldown)
		case providers.ErrorRate:
			if retryCounts[key] <= 2 {
				if err := workflow.Sleep(ctx, time.Duration(retryCounts[key]*2)*time.Second); err != nil {
					return activities.LLMGenerateOutput{}, string(providers.ClassifyError(err)), err
				}
				attempt--
			} else {
				disableProviderUntil(ctx, state, idx, 2*time.Minute)
			}
		case providers.ErrorTransient:
			if retryCounts[key] <= 2 {
				if err := workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second); err != nil {
					return activities.LLMGenerateOutput{}, string(providers.ClassifyError(err)), err
				}
				attempt--
			}
		case providers.ErrorContext:
			return activities.LLMGenerateOutput{}, string(providers.ErrorContext), err
		default:
			disableProviderUntil(ctx, state, idx, time.Minute)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all llm providers exhausted")
	}
	return activities.LLMGenerateOutput{}, string(providers.ClassifyError(lastErr)), lastErr
}

func isProviderDisabled(ctx workflow.Context, state *providerState, idx int) bool {
	until, ok := state.disabledUntil[idx]
	if !ok {
		return false
	}
	return workflow.Now(ctx).Before(until)
}

func disableProviderUntil(ctx workflow.Context, state *providerState, idx int, d time.Duration) {
	state.disabledUntil[idx] = workflow.Now(ctx).Add(d)
}

func defaultChunkVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "v1"
	}
	return v
}

func defaultEmbedVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "v0"
	}
	return v
}

func isNoTextError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no extractable text")
}

func isInvalidTextEncodingError(err error) bool {
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "invalid byte sequence") || strings.Contains(e, "sqlstate 22021")
}

func filepathBase(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}

type surveyReference struct {
	Key      string
	PaperID  string
	Title    string
	Authors  string
	Year     int
	Filename string
	ChunkIDs []string
}

func buildSurveyReferences(results []activities.SearchChunk) ([]surveyReference, []string) {
	refs := make([]surveyReference, 0)
	paperToIdx := map[string]int{}
	context := make([]string, 0, len(results))
	for _, c := range results {
		paperID := strings.TrimSpace(c.PaperID)
		if paperID == "" {
			paperID = c.ChunkID
		}
		idx, ok := paperToIdx[paperID]
		if !ok {
			idx = len(refs)
			paperToIdx[paperID] = idx
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = "Untitled Source"
			}
			refs = append(refs, surveyReference{
				Key:     fmt.Sprintf("ref%d", idx+1),
				PaperID: paperID,
				Title:   title,
			})
		}
		if c.ChunkID != "" {
			refs[idx].ChunkIDs = append(refs[idx].ChunkIDs, c.ChunkID)
		}
		context = append(context, fmt.Sprintf(
			"Source %s | Title: %s | Chunk: %s | Evidence: %s",
			refs[idx].Key,
			refs[idx].Title,
			c.ChunkID,
			latexSanitizeContext(c.Text),
		))
	}
	return refs, context
}

func latexSanitizeContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 1400 {
		s = s[:1400] + "..."
	}
	return s
}

func buildLatexPrompt(topic string, refs []surveyReference) string {
	refLines := make([]string, 0, len(refs))
	for _, ref := range refs {
		refLines = append(refLines, fmt.Sprintf("- %s: %s", ref.Key, ref.Title))
	}
	return strings.Join([]string{
		"Write a citation-grounded literature survey in LaTeX body format for this topic:",
		topic,
		"",
		"Output requirements:",
		"1. Output ONLY LaTeX content for the body (no \\documentclass, no bibliography environment, no code fences).",
		"2. Include exactly one \\section{Related Work}; do not create one section per individual paper.",
		"3. The Related Work section must synthesize papers thematically and compare methods/findings.",
		"4. Use inline citation keys like [ref1], [ref2] directly in text (do not use \\cite).",
		"5. Every source key listed below must appear at least once in the Related Work section.",
		"6. Every factual claim must cite one or more listed keys; do not cite any key outside this list.",
		"7. Do not include a bibliography or references section.",
		"8. If evidence is weak, explicitly state limitations.",
		"",
		"Allowed citation keys:",
		strings.Join(refLines, "\n"),
	}, "\n")
}

func cleanLLMDocument(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```latex")
	s = strings.TrimPrefix(s, "```tex")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if strings.Contains(s, "\\begin{document}") {
		if parts := strings.Split(s, "\\begin{document}"); len(parts) > 1 {
			s = parts[1]
		}
	}
	if strings.Contains(s, "\\end{document}") {
		if parts := strings.Split(s, "\\end{document}"); len(parts) > 0 {
			s = parts[0]
		}
	}
	return strings.TrimSpace(s)
}

func buildLatexDocument(topic string, refs []surveyReference, body string, generationFailed bool) string {
	var b strings.Builder
	b.WriteString("\\documentclass[conference]{IEEEtran}\n")
	b.WriteString("\\usepackage[hidelinks]{hyperref}\n\n")
	b.WriteString("\\title{Literature Survey: " + latexEscape(topic) + "}\n")
	b.WriteString("\\author{LitFlow Automated Draft}\n\n")
	b.WriteString("\\begin{document}\n")
	b.WriteString("\\maketitle\n\n")
	if strings.TrimSpace(body) == "" {
		b.WriteString("\\begin{abstract}\n")
		b.WriteString("This draft was generated from retrieved evidence but requires manual completion due to limited model output.\n")
		b.WriteString("\\end{abstract}\n\n")
		b.WriteString("\\section{Related Work}\n")
		b.WriteString("This section summarizes the retrieved conference literature for the topic and requires manual expansion.\n")
		b.WriteString("The current evidence pool includes " + inlineRefMentions(refs) + ".\n\n")
	} else {
		if !hasRelatedWorkSection(body) {
			b.WriteString("\\section{Related Work}\n")
			b.WriteString("This section synthesizes the retrieved conference literature for the topic. ")
			b.WriteString("Core references considered in this synthesis include " + inlineRefMentions(refs) + ".\n\n")
		}
		b.WriteString(body + "\n\n")
	}
	if generationFailed {
		b.WriteString("\\section*{Generation Note}\n")
		b.WriteString("Model generation encountered an issue; review and expand this draft manually.\n\n")
	}
	b.WriteString("\\section*{Source Papers}\n")
	b.WriteString("\\begin{itemize}\n")
	for _, ref := range refs {
		title := latexEscape(strings.TrimSpace(ref.Title))
		if title == "" {
			title = "Untitled paper"
		}
		b.WriteString("\\item [" + latexEscape(ref.Key) + "] " + title + "\n")
	}
	b.WriteString("\\end{itemize}\n\n")
	b.WriteString("\\end{document}\n")
	return b.String()
}

func hasRelatedWorkSection(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "\\section{related work}")
}

func inlineRefMentions(refs []surveyReference) string {
	if len(refs) == 0 {
		return "the retrieved sources"
	}
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, "["+ref.Key+"]")
	}
	return strings.Join(keys, ", ")
}

func latexEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`{`, `\{`,
		`}`, `\}`,
		`$`, `\$`,
		`&`, `\&`,
		`#`, `\#`,
		`_`, `\_`,
		`%`, `\%`,
		`~`, `\textasciitilde{}`,
		`^`, `\textasciicircum{}`,
	)
	return r.Replace(strings.TrimSpace(s))
}

func durationOrDefault(seconds int, fallback int) time.Duration {
	if seconds <= 0 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

func defaultCount(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func defaultSeconds(n int, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return n
}

func pathForBackfill(input BackfillInput, filename string) string {
	base := strings.TrimSpace(input.DataInRoot)
	if base == "" {
		base = "./data/in"
	}
	return filepath.Join(base, input.CorpusID, filename)
}
