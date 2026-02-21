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
	if err := workflow.ExecuteActivity(ctx, "ExtractMetadataActivity", activities.ExtractMetadataInput{Text: textOut.Text}).Get(ctx, &metaOut); err != nil {
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
	progress := SurveyProgress{SurveyRunID: input.SurveyRunID, CorpusID: input.CorpusID, TotalTopics: len(input.Topics), TopicStatus: map[string]string{}}
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

	report := strings.Builder{}
	report.WriteString("# Literature Survey\n\n")
	report.WriteString("Corpus: `" + input.CorpusID + "`\n\n")

	for _, topic := range input.Topics {
		progress.TopicStatus[topic] = "retrieving"
		eq, err := callEmbedQueryWithFailover(ctx, &embedState, embedProviders, cooldown, activities.EmbedQueryInput{Operation: "survey_topic_embed", Text: topic}, nil)
		if err != nil {
			progress.TopicStatus[topic] = "failed"
			continue
		}
		var retrieved activities.SearchChunksOutput
		if err := workflow.ExecuteActivity(ctx, "SearchChunksActivity", activities.SearchChunksInput{
			CorpusID:         input.CorpusID,
			QueryVec:         eq.Vector,
			TopK:             8,
			EmbeddingVersion: defaultEmbedVersion(input.EmbedVersion),
		}).Get(ctx, &retrieved); err != nil {
			progress.TopicStatus[topic] = "failed"
			continue
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
		progress.TopicStatus[topic] = "drafting"

		contextWindow := toCitationContext(retrieved.Results)
		outline, _, _ := callLLMWithFailover(ctx, &llmState, llmProviders, cooldown, activities.LLMGenerateInput{Operation: "survey_outline", CorpusID: input.CorpusID, Prompt: "Create an outline for topic: " + topic, Context: contextWindow}, nil)

		sectionInput := activities.LLMGenerateInput{Operation: "survey_section", CorpusID: input.CorpusID, Prompt: "Draft literature survey section for topic: " + topic, Context: contextWindow}
		section, sectionErrType, sectionErr := callLLMWithFailover(ctx, &llmState, llmProviders, cooldown, sectionInput, nil)
		if sectionErr != nil && sectionErrType == string(providers.ErrorContext) {
			reduced := contextWindow
			if len(reduced) > 3 {
				reduced = reduced[:3]
			}
			sectionInput.Context = reduced
			section, _, sectionErr = callLLMWithFailover(ctx, &llmState, llmProviders, cooldown, sectionInput, nil)
		}

		report.WriteString("## " + topic + "\n\n")
		if strings.TrimSpace(outline.Text) != "" {
			report.WriteString("### Outline\n")
			report.WriteString(outline.Text + "\n\n")
		}
		report.WriteString("### Discussion\n")
		if sectionErr != nil || strings.TrimSpace(section.Text) == "" {
			report.WriteString("No generated section text.\n\n")
		} else {
			report.WriteString(section.Text + "\n\n")
		}
		report.WriteString("### Citations\n")
		for _, c := range retrieved.Results {
			report.WriteString("- [" + c.Title + ":" + c.ChunkID + "] score=" + formatScore(c.Score) + "\n")
		}
		report.WriteString("\n")
		progress.TopicStatus[topic] = "done"
		progress.DoneTopics++
	}

	var reportOut activities.WriteSurveyReportOutput
	if err := workflow.ExecuteActivity(ctx, "WriteSurveyReportActivity", activities.WriteSurveyReportInput{CorpusID: input.CorpusID, SurveyRunID: input.SurveyRunID, Report: report.String()}).Get(ctx, &reportOut); err != nil {
		return "", err
	}
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
				workflow.Sleep(ctx, time.Duration(retryCounts[key]*2)*time.Second)
				if !strict {
					attempt--
				}
			} else {
				disableProviderUntil(ctx, state, idx, 2*time.Minute)
			}
		case providers.ErrorTransient:
			if retryCounts[key] <= 2 {
				workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second)
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
				workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second)
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

func callLLMWithFailover(ctx workflow.Context, state *providerState, providerCount int, cooldown time.Duration, input activities.LLMGenerateInput, retryCounts map[string]int) (activities.LLMGenerateOutput, string, error) {
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
		var out activities.LLMGenerateOutput
		err := workflow.ExecuteActivity(ctx, "LLMGenerateActivity", input).Get(ctx, &out)
		if err == nil {
			_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: out.ProviderName, Model: out.Model, RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "ok"}).Get(ctx, nil)
			return out, "", nil
		}
		lastErr = err
		errType := providers.ClassifyError(err)
		_ = workflow.ExecuteActivity(ctx, "LogLLMCallActivity", activities.LogLLMCallInput{Operation: input.Operation, CorpusID: input.CorpusID, PaperID: input.PaperID, ProviderName: fmt.Sprintf("provider-%d", idx), RequestID: fmt.Sprintf("%s-%d", input.Operation, attempt), Status: "failed", ErrorType: string(errType)}).Get(ctx, nil)
		key := fmt.Sprintf("llm-%s-%d", input.Operation, idx)
		retryCounts[key]++
		switch errType {
		case providers.ErrorQuota:
			disableProviderUntil(ctx, state, idx, cooldown)
		case providers.ErrorRate:
			if retryCounts[key] <= 2 {
				workflow.Sleep(ctx, time.Duration(retryCounts[key]*2)*time.Second)
				attempt--
			} else {
				disableProviderUntil(ctx, state, idx, 2*time.Minute)
			}
		case providers.ErrorTransient:
			if retryCounts[key] <= 2 {
				workflow.Sleep(ctx, time.Duration(retryCounts[key])*time.Second)
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

func formatScore(v float64) string {
	return fmt.Sprintf("%.4f", v)
}

func toCitationContext(results []activities.SearchChunk) []string {
	out := make([]string, 0, len(results))
	for _, c := range results {
		out = append(out, fmt.Sprintf("[%s:%s] %s", c.Title, c.ChunkID, c.Text))
	}
	return out
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
