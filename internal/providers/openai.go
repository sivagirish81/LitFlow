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

// OpenAIProvider uses standard OpenAI REST APIs when keys are configured.
type OpenAIProvider struct {
	keyName string
	apiKey  string
	client  *http.Client
}

func NewOpenAIProvider(keyName string) *OpenAIProvider {
	apiKey := resolveOpenAIKey(keyName)
	return &OpenAIProvider{
		keyName: keyName,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OpenAIProvider) Embed(ctx context.Context, req EmbedRequest) ([][]float32, ProviderInfo, error) {
	if o.apiKey == "" {
		return nil, ProviderInfo{Name: "openai", Key: o.keyName}, fmt.Errorf("openai key missing for alias %q", o.keyName)
	}
	model := "text-embedding-3-small"
	payload, _ := json.Marshal(map[string]any{"model": model, "input": req.Inputs})
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("openai embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("openai embedding error %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("decode embedding response: %w", err)
	}
	out := make([][]float32, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		out = append(out, d.Embedding)
	}
	return out, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, nil
}

func (o *OpenAIProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, ProviderInfo, error) {
	if o.apiKey == "" {
		return GenerateResponse{}, ProviderInfo{Name: "openai", Key: o.keyName}, fmt.Errorf("openai key missing for alias %q", o.keyName)
	}
	model := "gpt-4o-mini"
	prompt := req.Prompt
	if len(req.Context) > 0 {
		prompt = prompt + "\n\nContext:\n" + strings.Join(req.Context, "\n\n")
	}
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a literature survey assistant. Use concise, citation-grounded responses."},
			{"role": "user", "content": prompt},
		},
	})
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("openai generate request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return GenerateResponse{}, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("openai generate error %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return GenerateResponse{}, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("decode generate response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return GenerateResponse{}, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, fmt.Errorf("openai returned empty choices")
	}
	return GenerateResponse{Text: parsed.Choices[0].Message.Content}, ProviderInfo{Name: "openai", Model: model, Key: o.keyName}, nil
}

func resolveOpenAIKey(alias string) string {
	if alias != "" {
		k := os.Getenv("LITFLOW_OPENAI_KEY_" + strings.ToUpper(alias))
		if k != "" {
			return k
		}
	}
	return os.Getenv("OPENAI_API_KEY")
}
