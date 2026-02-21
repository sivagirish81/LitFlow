package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OllamaEmbeddingProvider supports local, free embeddings via Ollama.
// Example model: nomic-embed-text (Nomic Embed v1.5 family).
type OllamaEmbeddingProvider struct {
	alias   string
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaEmbeddingProvider(alias string) *OllamaEmbeddingProvider {
	baseURL := strings.TrimSpace(os.Getenv("LITFLOW_OLLAMA_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := resolveOllamaEmbedModel(alias)
	return &OllamaEmbeddingProvider{
		alias:   alias,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (o *OllamaEmbeddingProvider) Embed(ctx context.Context, req EmbedRequest) ([][]float32, ProviderInfo, error) {
	if len(req.Inputs) == 0 {
		return nil, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, fmt.Errorf("no embedding inputs")
	}
	out := make([][]float32, 0, len(req.Inputs))
	for _, text := range req.Inputs {
		payload, _ := json.Marshal(map[string]any{
			"model":  o.model,
			"prompt": text,
		})
		httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embeddings", bytes.NewReader(payload))
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := o.client.Do(httpReq)
		if err != nil {
			return nil, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, fmt.Errorf("ollama embedding request failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, fmt.Errorf("ollama embedding error %d: %s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, fmt.Errorf("decode ollama embedding response: %w", err)
		}
		if len(parsed.Embedding) == 0 {
			return nil, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, fmt.Errorf("ollama returned empty embedding")
		}
		out = append(out, matchDimension(parsed.Embedding, req.Dimension))
	}
	return out, ProviderInfo{Name: "ollama", Model: o.model, Key: o.alias}, nil
}

func resolveOllamaEmbedModel(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias != "" {
		key := "LITFLOW_OLLAMA_EMBED_MODEL_" + strings.ToUpper(sanitizeEnvToken(alias))
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		switch strings.ToLower(alias) {
		case "nomic":
			return "nomic-embed-text"
		case "bge":
			return "bge-small-en-v1.5"
		}
		// Allow direct model in provider list, e.g. ollama:nomic-embed-text
		if strings.Contains(alias, "-") || strings.Contains(alias, "/") || strings.Contains(alias, ".") {
			return alias
		}
	}
	if v := strings.TrimSpace(os.Getenv("LITFLOW_OLLAMA_EMBED_MODEL")); v != "" {
		return v
	}
	return "nomic-embed-text"
}

func ResolveOllamaEmbedModel(alias string) string {
	return resolveOllamaEmbedModel(alias)
}

func sanitizeEnvToken(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func matchDimension(v []float32, target int) []float32 {
	if target <= 0 || len(v) == target {
		return v
	}
	if len(v) > target {
		return v[:target]
	}
	out := make([]float32, target)
	copy(out, v)
	return out
}
