package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"crow.watch/internal/dotenv"
	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dotenv.Load(".env")

	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	updated, err := queries.RecalculateStoryScores(ctx)
	if err != nil {
		log.Fatalf("recalculate scores: %v", err)
	}

	fmt.Printf("Recalculated scores for %d stories.\n", updated)
}
