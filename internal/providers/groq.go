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

// GroqProvider supports LLM generation via Groq's OpenAI-compatible API.
type GroqProvider struct {
	keyName string
	apiKey  string
	model   string
	client  *http.Client
}

func NewGroqProvider(keyName string) *GroqProvider {
	model := os.Getenv("LITFLOW_GROQ_MODEL")
	if strings.TrimSpace(model) == "" {
		model = "llama-3.1-8b-instant"
	}
	return &GroqProvider{
		keyName: keyName,
		apiKey:  resolveGroqKey(keyName),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GroqProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, ProviderInfo, error) {
	if g.apiKey == "" {
		return GenerateResponse{}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, fmt.Errorf("groq key missing for alias %q", g.keyName)
	}
	prompt := req.Prompt
	if len(req.Context) > 0 {
		prompt += "\n\nContext:\n" + strings.Join(req.Context, "\n\n")
	}
	payload, _ := json.Marshal(map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a literature survey assistant. Keep responses concise and grounded in provided context."},
			{"role": "user", "content": prompt},
		},
	})
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, fmt.Errorf("groq generate request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return GenerateResponse{}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, fmt.Errorf("groq generate error %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return GenerateResponse{}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, fmt.Errorf("decode groq response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return GenerateResponse{}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, fmt.Errorf("groq returned empty choices")
	}
	return GenerateResponse{Text: parsed.Choices[0].Message.Content}, ProviderInfo{Name: "groq", Key: g.keyName, Model: g.model}, nil
}

func resolveGroqKey(alias string) string {
	if alias != "" {
		if v := os.Getenv("LITFLOW_GROQ_KEY_" + strings.ToUpper(alias)); v != "" {
			return v
		}
	}
	return os.Getenv("GROQ_API_KEY")
}
