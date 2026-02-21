package workflows

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"litflow/internal/activities"
	"litflow/internal/graph"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	QueryGetKGBackfillProgress = "GetKGBackfillProgress"
)

func KGBackfillWorkflow(ctx workflow.Context, input KGBackfillInput) (string, error) {
	progress := KGBackfillProgress{CorpusID: input.CorpusID, PerPaper: map[string]string{}}
	if err := workflow.SetQueryHandler(ctx, QueryGetKGBackfillProgress, func() (KGBackfillProgress, error) { return progress, nil }); err != nil {
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

	var papers activities.ListCorpusPapersOutput
	if err := workflow.ExecuteActivity(ctx, "ListCorpusPapersActivity", activities.ListCorpusPapersInput{CorpusID: input.CorpusID}).Get(ctx, &papers); err != nil {
		return "", err
	}
	progress.Total = len(papers.Papers)
	maxC := input.MaxConcurrent
	if maxC <= 0 {
		maxC = 4
	}
	for i := 0; i < len(papers.Papers); i += maxC {
		end := i + maxC
		if end > len(papers.Papers) {
			end = len(papers.Papers)
		}
		futures := make([]workflow.ChildWorkflowFuture, 0, end-i)
		pids := make([]string, 0, end-i)
		for _, p := range papers.Papers[i:end] {
			progress.PerPaper[p.PaperID] = "running"
			f := workflow.ExecuteChildWorkflow(ctx, KGExtractPaperWorkflow, KGExtractPaperInput{
				CorpusID:        input.CorpusID,
				PaperID:         p.PaperID,
				PromptVersion:   defaultPromptVersion(input.PromptVersion),
				ModelVersion:    defaultModelVersion(input.ModelVersion),
				LLMProviders:    defaultCount(input.LLMProviders),
				LLMProviderRefs: input.LLMProviderRefs,
				CooldownSeconds: defaultSeconds(input.CooldownSeconds, 900),
			})
			futures = append(futures, f)
			pids = append(pids, p.PaperID)
		}
		for i := range futures {
			var st string
			if err := futures[i].Get(ctx, &st); err != nil {
				progress.Failed++
				progress.PerPaper[pids[i]] = "failed"
				continue
			}
			if st != "completed" {
				progress.Failed++
			}
			progress.Done++
			progress.PerPaper[pids[i]] = st
		}
	}
	return "completed", nil
}

func KGExtractPaperWorkflow(ctx workflow.Context, input KGExtractPaperInput) (string, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    20 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	promptHash := hashPrompt(defaultPromptVersion(input.PromptVersion))
	_ = workflow.ExecuteActivity(ctx, "MarkKGPaperRunActivity", activities.MarkKGPaperRunInput{
		CorpusID:     input.CorpusID,
		PaperID:      input.PaperID,
		PromptHash:   promptHash,
		ModelVersion: defaultModelVersion(input.ModelVersion),
		Status:       "running",
	}).Get(ctx, nil)

	var chunkOut activities.ListPaperChunksOutput
	if err := workflow.ExecuteActivity(ctx, "ListPaperChunksActivity", activities.KGPaperInput{CorpusID: input.CorpusID, PaperID: input.PaperID}).Get(ctx, &chunkOut); err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkKGPaperRunActivity", activities.MarkKGPaperRunInput{
			CorpusID:     input.CorpusID,
			PaperID:      input.PaperID,
			PromptHash:   promptHash,
			ModelVersion: defaultModelVersion(input.ModelVersion),
			Status:       "failed",
			LastError:    err.Error(),
		}).Get(ctx, nil)
		return "failed", nil
	}

	state := newProviderState()
	triples := make([]activities.KGTripleRecord, 0, 64)
	llmFailures := 0
	lastLLMErr := ""
	for _, c := range chunkOut.Chunks {
		prompt := graph.BuildChunkExtractionPrompt(chunkOut.Title, c.Text)
		resp, _, llmErr := callLLMWithFailover(ctx, &state, defaultCount(input.LLMProviders), input.LLMProviderRefs, durationOrDefault(input.CooldownSeconds, 900), activities.LLMGenerateInput{
			Operation: "kg_extract",
			CorpusID:  input.CorpusID,
			PaperID:   input.PaperID,
			Prompt:    prompt,
		}, nil)
		if llmErr != nil {
			llmFailures++
			lastLLMErr = llmErr.Error()
			continue
		}
		parsed := graph.ParseTriplesJSON(resp.Text)
		for _, t := range parsed {
			triples = append(triples, activities.ToKGRecord(t, c.ChunkID))
		}
	}
	if len(chunkOut.Chunks) > 0 && llmFailures == len(chunkOut.Chunks) {
		_ = workflow.ExecuteActivity(ctx, "MarkKGPaperRunActivity", activities.MarkKGPaperRunInput{
			CorpusID:     input.CorpusID,
			PaperID:      input.PaperID,
			PromptHash:   promptHash,
			ModelVersion: defaultModelVersion(input.ModelVersion),
			Status:       "failed",
			TripleCount:  0,
			LastError:    "kg extraction exhausted all llm providers: " + lastLLMErr,
		}).Get(ctx, nil)
		return "failed", nil
	}

	if err := workflow.ExecuteActivity(ctx, "UpsertKGTriplesActivity", activities.UpsertKGTriplesInput{
		CorpusID:     input.CorpusID,
		PaperID:      input.PaperID,
		PromptHash:   promptHash,
		ModelVersion: defaultModelVersion(input.ModelVersion),
		Triples:      triples,
	}).Get(ctx, nil); err != nil {
		_ = workflow.ExecuteActivity(ctx, "MarkKGPaperRunActivity", activities.MarkKGPaperRunInput{
			CorpusID:     input.CorpusID,
			PaperID:      input.PaperID,
			PromptHash:   promptHash,
			ModelVersion: defaultModelVersion(input.ModelVersion),
			Status:       "failed",
			TripleCount:  len(triples),
			LastError:    err.Error(),
		}).Get(ctx, nil)
		return "failed", nil
	}
	_ = workflow.ExecuteActivity(ctx, "MarkKGPaperRunActivity", activities.MarkKGPaperRunInput{
		CorpusID:     input.CorpusID,
		PaperID:      input.PaperID,
		PromptHash:   promptHash,
		ModelVersion: defaultModelVersion(input.ModelVersion),
		Status:       "completed",
		TripleCount:  len(triples),
	}).Get(ctx, nil)
	return "completed", nil
}

func defaultPromptVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "v1"
	}
	return v
}

func defaultModelVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "kg-llm-v1"
	}
	return v
}

func hashPrompt(v string) string {
	x := sha256.Sum256([]byte(v))
	return hex.EncodeToString(x[:])
}
