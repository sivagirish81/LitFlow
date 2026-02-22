package main

import (
	"log"
	"net/http"

	"litflow/internal/api"
	"litflow/internal/config"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	cfg := config.Load()
	h := api.NewServer(cfg)
	log.Printf("litflow api listening on %s llm_providers=%q embed_providers=%q", cfg.APIAddr, cfg.LLMProviders, cfg.EmbedProviders)
	if err := http.ListenAndServe(cfg.APIAddr, h.Routes()); err != nil {
		log.Fatal(err)
	}
}
