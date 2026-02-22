package activities

import "go.temporal.io/sdk/worker"

func Register(w worker.Worker, a *Activities) {
	w.RegisterActivity(a.ListPDFsActivity)
	w.RegisterActivity(a.WriteCorpusSummaryActivity)
	w.RegisterActivity(a.ListFailedPapersActivity)
	w.RegisterActivity(a.ListCorpusPapersActivity)
	w.RegisterActivity(a.WriteRunManifestActivity)
	w.RegisterActivity(a.ComputePaperIDActivity)
	w.RegisterActivity(a.ExtractTextActivity)
	w.RegisterActivity(a.ExtractMetadataActivity)
	w.RegisterActivity(a.ChunkTextActivity)
	w.RegisterActivity(a.UpsertChunksActivity)
	w.RegisterActivity(a.WritePaperArtifactsActivity)
	w.RegisterActivity(a.UpdatePaperStatusActivity)
	w.RegisterActivity(a.EmbedChunksActivity)
	w.RegisterActivity(a.LLMGenerateActivity)
	w.RegisterActivity(a.EmbedQueryActivity)
	w.RegisterActivity(a.SearchChunksActivity)
	w.RegisterActivity(a.WriteSurveyReportActivity)
	w.RegisterActivity(a.UpdateSurveyRunActivity)
	w.RegisterActivity(a.LogLLMCallActivity)
	w.RegisterActivity(a.UpsertTopicGraphActivity)
	w.RegisterActivity(a.ListPaperChunksActivity)
	w.RegisterActivity(a.UpsertKGTriplesActivity)
	w.RegisterActivity(a.MarkKGPaperRunActivity)
}
