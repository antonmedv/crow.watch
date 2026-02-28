package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	loadDotEnv(".env")

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

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		_ = os.Setenv(key, strings.Trim(value, `"'`))
	}
}
