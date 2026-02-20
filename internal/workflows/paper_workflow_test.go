package workflows

import (
	"context"
	"errors"
	"testing"

	"litflow/internal/activities"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func registerActivityName[T any](env *testsuite.TestWorkflowEnvironment, name string, fn T) {
	env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name})
}

func TestPaperProcessWorkflowSuccess(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(PaperProcessWorkflow)
	registerActivityName(env, "ComputePaperIDActivity", func(context.Context, activities.ComputePaperIDInput) (activities.ComputePaperIDOutput, error) {
		return activities.ComputePaperIDOutput{}, nil
	})
	registerActivityName(env, "UpdatePaperStatusActivity", func(context.Context, activities.UpdatePaperStatusInput) error { return nil })
	registerActivityName(env, "ExtractTextActivity", func(context.Context, activities.ExtractTextInput) (activities.ExtractTextOutput, error) {
		return activities.ExtractTextOutput{}, nil
	})
	registerActivityName(env, "ExtractMetadataActivity", func(context.Context, activities.ExtractMetadataInput) (activities.ExtractMetadataOutput, error) {
		return activities.ExtractMetadataOutput{}, nil
	})
	registerActivityName(env, "ChunkTextActivity", func(context.Context, activities.ChunkTextInput) (activities.ChunkTextOutput, error) {
		return activities.ChunkTextOutput{}, nil
	})
	registerActivityName(env, "EmbedChunksActivity", func(context.Context, activities.EmbedChunksInput) (activities.EmbedChunksOutput, error) {
		return activities.EmbedChunksOutput{}, nil
	})
	registerActivityName(env, "UpsertChunksActivity", func(context.Context, activities.UpsertChunksInput) error { return nil })
	registerActivityName(env, "WritePaperArtifactsActivity", func(context.Context, activities.WritePaperArtifactsInput) error { return nil })
	registerActivityName(env, "LogLLMCallActivity", func(context.Context, activities.LogLLMCallInput) error { return nil })

	env.OnActivity("ComputePaperIDActivity", mock.Anything, activities.ComputePaperIDInput{PaperPath: "/tmp/p.pdf"}).Return(activities.ComputePaperIDOutput{PaperID: "paper123"}, nil)
	env.OnActivity("UpdatePaperStatusActivity", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("ExtractTextActivity", mock.Anything, activities.ExtractTextInput{PaperPath: "/tmp/p.pdf"}).Return(activities.ExtractTextOutput{Text: "title\nauthor\ntext body"}, nil)
	env.OnActivity("ExtractMetadataActivity", mock.Anything, activities.ExtractMetadataInput{Text: "title\nauthor\ntext body"}).Return(activities.ExtractMetadataOutput{Title: "title", Authors: "author"}, nil)
	env.OnActivity("ChunkTextActivity", mock.Anything, mock.Anything).Return(activities.ChunkTextOutput{Chunks: []activities.ChunkItem{{ChunkID: "c1", PaperID: "paper123", CorpusID: "c", ChunkIndex: 0, Text: "chunk"}}}, nil)
	env.OnActivity("EmbedChunksActivity", mock.Anything, mock.Anything).Return(activities.EmbedChunksOutput{Vectors: [][]float32{{0.1, 0.2}}, ProviderName: "mock", Model: "mock"}, nil)
	env.OnActivity("UpsertChunksActivity", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("WritePaperArtifactsActivity", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("LogLLMCallActivity", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(PaperProcessWorkflow, PaperProcessInput{CorpusID: "c", PaperPath: "/tmp/p.pdf", EmbedProviders: 1, CooldownSeconds: 10})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out string
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, "processed", out)
}

func TestPaperProcessWorkflowNoTextFailsGracefully(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(PaperProcessWorkflow)
	registerActivityName(env, "ComputePaperIDActivity", func(context.Context, activities.ComputePaperIDInput) (activities.ComputePaperIDOutput, error) {
		return activities.ComputePaperIDOutput{}, nil
	})
	registerActivityName(env, "UpdatePaperStatusActivity", func(context.Context, activities.UpdatePaperStatusInput) error { return nil })
	registerActivityName(env, "ExtractTextActivity", func(context.Context, activities.ExtractTextInput) (activities.ExtractTextOutput, error) {
		return activities.ExtractTextOutput{}, nil
	})

	env.OnActivity("ComputePaperIDActivity", mock.Anything, mock.Anything).Return(activities.ComputePaperIDOutput{PaperID: "paper123"}, nil)
	env.OnActivity("UpdatePaperStatusActivity", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("ExtractTextActivity", mock.Anything, mock.Anything).Return(activities.ExtractTextOutput{}, errors.New("no extractable text found in PDF"))

	env.ExecuteWorkflow(PaperProcessWorkflow, PaperProcessInput{CorpusID: "c", PaperPath: "/tmp/p.pdf", EmbedProviders: 1, CooldownSeconds: 10})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var out string
	require.NoError(t, env.GetWorkflowResult(&out))
	require.Equal(t, "failed", out)
}
