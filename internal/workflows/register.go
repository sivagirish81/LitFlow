package workflows

import "go.temporal.io/sdk/worker"

func Register(w worker.Worker) {
	w.RegisterWorkflow(CorpusIngestWorkflow)
	w.RegisterWorkflow(PaperProcessWorkflow)
	w.RegisterWorkflow(SurveyBuildWorkflow)
	w.RegisterWorkflow(BackfillWorkflow)
}
