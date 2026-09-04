package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Mightyfin/decision-engine/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	url := os.Getenv("DECISION_ENGINE_DATABASE_URL")
	if url == "" {
		log.Fatal("DECISION_ENGINE_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Up(ctx, pool); err != nil {
		log.Fatal(err)
	}
}
