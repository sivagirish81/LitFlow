package providers

import (
	"fmt"
	"strings"

	"litflow/internal/config"
)

type NamedLLMProvider struct {
	Ref      ProviderRef
	Provider LLMProvider
}

type NamedEmbedProvider struct {
	Ref      ProviderRef
	Provider EmbeddingProvider
}

type Manager struct {
	llmProviders   []NamedLLMProvider
	embedProviders []NamedEmbedProvider
}

func NewManager(cfg config.Config) (*Manager, error) {
	llmRefs := ParseProviderList(cfg.LLMProviders)
	embedRefs := ParseProviderList(cfg.EmbedProviders)

	m := &Manager{}
	for _, ref := range llmRefs {
		p, err := buildProvider(ref, cfg.EmbedDim)
		if err != nil {
			return nil, err
		}
		llm, ok := p.(LLMProvider)
		if !ok {
			return nil, fmt.Errorf("provider %s does not support llm", ref.Raw)
		}
		m.llmProviders = append(m.llmProviders, NamedLLMProvider{Ref: ref, Provider: llm})
	}
	for _, ref := range embedRefs {
		p, err := buildProvider(ref, cfg.EmbedDim)
		if err != nil {
			return nil, err
		}
		embed, ok := p.(EmbeddingProvider)
		if !ok {
			return nil, fmt.Errorf("provider %s does not support embeddings", ref.Raw)
		}
		m.embedProviders = append(m.embedProviders, NamedEmbedProvider{Ref: ref, Provider: embed})
	}
	if len(m.embedProviders) == 0 {
		m.embedProviders = []NamedEmbedProvider{{Ref: ProviderRef{Raw: "mock", Name: "mock"}, Provider: NewMockProvider(cfg.EmbedDim)}}
	}
	if len(m.llmProviders) == 0 {
		m.llmProviders = []NamedLLMProvider{{Ref: ProviderRef{Raw: "mock", Name: "mock"}, Provider: NewMockProvider(cfg.EmbedDim)}}
	}
	return m, nil
}

func (m *Manager) FirstEmbedProvider() EmbeddingProvider {
	return m.embedProviders[0].Provider
}

func (m *Manager) FirstLLMProvider() LLMProvider {
	return m.llmProviders[0].Provider
}

func (m *Manager) EmbedProviderByIndex(i int) (EmbeddingProvider, ProviderRef) {
	if len(m.embedProviders) == 0 {
		p := NewMockProvider(1536)
		return p, ProviderRef{Raw: "mock", Name: "mock"}
	}
	if i < 0 || i >= len(m.embedProviders) {
		i = 0
	}
	return m.embedProviders[i].Provider, m.embedProviders[i].Ref
}

func (m *Manager) LLMProviderByIndex(i int) (LLMProvider, ProviderRef) {
	if len(m.llmProviders) == 0 {
		p := NewMockProvider(1536)
		return p, ProviderRef{Raw: "mock", Name: "mock"}
	}
	if i < 0 || i >= len(m.llmProviders) {
		i = 0
	}
	return m.llmProviders[i].Provider, m.llmProviders[i].Ref
}

func (m *Manager) EmbedCount() int {
	return len(m.embedProviders)
}

func (m *Manager) LLMCount() int {
	return len(m.llmProviders)
}

func (m *Manager) PreferredLLMOrder() []int {
	return preferredOrder(len(m.llmProviders), func(i int) string { return strings.ToLower(m.llmProviders[i].Ref.Name) })
}

func (m *Manager) PreferredEmbedOrder() []int {
	return preferredOrder(len(m.embedProviders), func(i int) string { return strings.ToLower(m.embedProviders[i].Ref.Name) })
}

func preferredOrder(n int, nameAt func(i int) string) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if nameAt(i) != "mock" {
			out = append(out, i)
		}
	}
	for i := 0; i < n; i++ {
		if nameAt(i) == "mock" {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		for i := 0; i < n; i++ {
			out = append(out, i)
		}
	}
	return out
}

func (m *Manager) FindLLMProviderByName(name string) (LLMProvider, ProviderRef, bool) {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil, ProviderRef{}, false
	}
	for i := range m.llmProviders {
		if strings.ToLower(m.llmProviders[i].Ref.Name) == target {
			return m.llmProviders[i].Provider, m.llmProviders[i].Ref, true
		}
	}
	return nil, ProviderRef{}, false
}

func (m *Manager) EmbedProviderRefs() []ProviderRef {
	out := make([]ProviderRef, 0, len(m.embedProviders))
	for i := range m.embedProviders {
		out = append(out, m.embedProviders[i].Ref)
	}
	return out
}

func (m *Manager) FindEmbedProviderIndex(raw string) int {
	target := strings.ToLower(strings.TrimSpace(raw))
	if target == "" {
		return -1
	}
	for i := range m.embedProviders {
		ref := m.embedProviders[i].Ref
		candidates := []string{
			strings.ToLower(strings.TrimSpace(ref.Raw)),
			strings.ToLower(strings.TrimSpace(ref.Name)),
		}
		if ref.KeyAlias != "" {
			candidates = append(candidates, strings.ToLower(strings.TrimSpace(ref.Name+":"+ref.KeyAlias)))
		}
		for _, c := range candidates {
			if c == target {
				return i
			}
		}
	}
	return -1
}

func buildProvider(ref ProviderRef, dim int) (any, error) {
	switch strings.ToLower(ref.Name) {
	case "mock":
		return NewMockProvider(dim), nil
	case "openai":
		return NewOpenAIProvider(ref.KeyAlias), nil
	case "ollama":
		return NewOllamaEmbeddingProvider(ref.KeyAlias), nil
	case "groq":
		return NewGroqProvider(ref.KeyAlias), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", ref.Name)
	}
}
