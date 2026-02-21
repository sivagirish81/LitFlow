package config

import (
	"os"
	"strconv"
)

type Config struct {
	APIAddr              string
	TemporalAddress      string
	TemporalTaskQueue    string
	PostgresURL          string
	DataInRoot           string
	DataOutRoot          string
	ChunkSize            int
	ChunkOverlap         int
	ProviderCooldownSecs int
	EmbedDim             int
	EmbedVersion         string
	WebAPIBase           string
	LLMProviders         string
	EmbedProviders       string
	IngestMaxChildren    int
}

func Load() Config {
	return Config{
		APIAddr:              getenv("LITFLOW_API_ADDR", ":8080"),
		TemporalAddress:      getenv("LITFLOW_TEMPORAL_ADDRESS", "localhost:7233"),
		TemporalTaskQueue:    getenv("LITFLOW_TEMPORAL_TASK_QUEUE", "litflow"),
		PostgresURL:          getenv("LITFLOW_POSTGRES_URL", "postgres://litflow:litflow@localhost:5432/litflow?sslmode=disable"),
		DataInRoot:           getenv("LITFLOW_DATA_IN", "./data/in"),
		DataOutRoot:          getenv("LITFLOW_DATA_OUT", "./data/out"),
		ChunkSize:            getenvInt("LITFLOW_CHUNK_SIZE", 1200),
		ChunkOverlap:         getenvInt("LITFLOW_CHUNK_OVERLAP", 200),
		ProviderCooldownSecs: getenvInt("LITFLOW_PROVIDER_COOLDOWN_SECONDS", 900),
		EmbedDim:             getenvInt("LITFLOW_EMBED_DIM", 1536),
		EmbedVersion:         getenv("LITFLOW_EMBED_VERSION", "v1"),
		WebAPIBase:           getenv("NEXT_PUBLIC_LITFLOW_API_BASE", "http://localhost:8080"),
		LLMProviders:         getenv("LITFLOW_LLM_PROVIDERS", "mock"),
		EmbedProviders:       getenv("LITFLOW_EMBED_PROVIDERS", "mock"),
		IngestMaxChildren:    getenvInt("LITFLOW_INGEST_MAX_CHILDREN", 3),
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func getenvInt(k string, fallback int) int {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
