package main

import (
	"context"
	"log"
	"time"

	"litflow/internal/activities"
	"litflow/internal/config"
	"litflow/internal/storage"
	"litflow/internal/workflows"

	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	_ = godotenv.Load(".env")
	cfg := config.Load()
	c, err := client.Dial(client.Options{HostPort: cfg.TemporalAddress})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	w := worker.New(c, cfg.TemporalTaskQueue, worker.Options{})
	workflows.Register(w)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := storage.NewDB(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	a, err := activities.New(cfg, db)
	if err != nil {
		log.Fatal(err)
	}
	activities.Register(w, a)

	log.Printf("litflow worker listening on %s queue=%s llm_providers=%q embed_providers=%q", cfg.TemporalAddress, cfg.TemporalTaskQueue, cfg.LLMProviders, cfg.EmbedProviders)
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatal(err)
	}
}
