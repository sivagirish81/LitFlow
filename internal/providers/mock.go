package providers

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

type MockProvider struct {
	dim int
}

func NewMockProvider(dim int) *MockProvider {
	if dim <= 0 {
		dim = 1536
	}
	return &MockProvider{dim: dim}
}

func (m *MockProvider) Embed(ctx context.Context, req EmbedRequest) ([][]float32, ProviderInfo, error) {
	_ = ctx
	dim := req.Dimension
	if dim <= 0 {
		dim = m.dim
	}
	vectors := make([][]float32, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		vectors = append(vectors, deterministicVector(input, dim))
	}
	return vectors, ProviderInfo{Name: "mock", Model: fmt.Sprintf("mock-embed-%d", dim), Key: "mock"}, nil
}

func (m *MockProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, ProviderInfo, error) {
	_ = ctx
	text := "Mock response."
	if strings.Contains(strings.ToLower(req.Operation), "survey") {
		text = "# Mock Survey\n\n## Findings\nDeterministic section output with citations [mock-paper:chunk-0]."
	} else if strings.Contains(strings.ToLower(req.Operation), "rag") || strings.Contains(strings.ToLower(req.Operation), "ask") {
		builder := strings.Builder{}
		builder.WriteString("## Direct Answer\n")
		builder.WriteString("- Deterministic answer based on retrieved evidence.")
		for i := range req.Context {
			builder.WriteString(" [C")
			builder.WriteString(strconv.Itoa(i + 1))
			builder.WriteString("]")
		}
		builder.WriteString("\n## Confidence\n- Mock confidence only; replace with real provider for semantic quality.")
		text = builder.String()
	} else if strings.Contains(strings.ToLower(req.Operation), "citation_summary") {
		text = "This citation is relevant to the question and provides supporting context. Interpret with caution because this is deterministic mock output."
	}
	return GenerateResponse{Text: text}, ProviderInfo{Name: "mock", Model: "mock-llm-v1", Key: "mock"}, nil
}

func deterministicVector(input string, dim int) []float32 {
	vec := make([]float32, dim)
	seed := []byte(input)
	if len(seed) == 0 {
		seed = []byte("empty")
	}
	for i := 0; i < dim; i++ {
		h := sha256.Sum256(append(seed, byte(i%251)))
		u := binary.BigEndian.Uint32(h[:4])
		v := float32(u%2000)/1000.0 - 1.0
		vec[i] = v
	}
	return normalize(vec)
}

func normalize(v []float32) []float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	inv := float32(1.0 / (float64(sum) + 1e-9))
	for i := range v {
		v[i] *= inv
	}
	return v
}
