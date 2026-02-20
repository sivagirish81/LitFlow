package providers

import "context"

type ProviderInfo struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Key   string `json:"key"`
}

type GenerateRequest struct {
	Operation string   `json:"operation"`
	Prompt    string   `json:"prompt"`
	Context   []string `json:"context"`
}

type GenerateResponse struct {
	Text string `json:"text"`
}

type EmbedRequest struct {
	Operation string   `json:"operation"`
	Inputs    []string `json:"inputs"`
	Dimension int      `json:"dimension"`
}

type LLMProvider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, ProviderInfo, error)
}

type EmbeddingProvider interface {
	Embed(ctx context.Context, req EmbedRequest) ([][]float32, ProviderInfo, error)
}
