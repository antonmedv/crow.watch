package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"crow.watch/internal/analytics"
	"crow.watch/internal/dotenv"
	"crow.watch/internal/store"
)

func main() {
	dotenv.Load(".env")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: analytics <aggregate|purge>\n")
		fmt.Fprintf(os.Stderr, "\n  aggregate          aggregate yesterday's page views\n")
		fmt.Fprintf(os.Stderr, "  aggregate <date>   aggregate a specific date (YYYY-MM-DD)\n")
		fmt.Fprintf(os.Stderr, "  purge              delete page views older than 90 days\n")
		os.Exit(1)
	}

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

	switch os.Args[1] {
	case "aggregate":
		cmdAggregate(ctx, queries, os.Args[2:])
	case "purge":
		cmdPurge(ctx, queries)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdAggregate(ctx context.Context, queries *store.Queries, args []string) {
	var date time.Time
	if len(args) > 0 {
		var err error
		date, err = time.Parse("2006-01-02", args[0])
		if err != nil {
			log.Fatalf("invalid date: %v", err)
		}
	} else {
		date = time.Now().UTC().AddDate(0, 0, -1)
	}

	dateStr := date.Format("2006-01-02")
	fmt.Printf("Aggregating analytics for %s...\n", dateStr)

	if err := analytics.Aggregate(ctx, queries, date); err != nil {
		log.Fatalf("aggregate: %v", err)
	}

	fmt.Printf("Done.\n")
}

func cmdPurge(ctx context.Context, queries *store.Queries) {
	const retention = 90 * 24 * time.Hour
	fmt.Printf("Purging page views older than 90 days...\n")

	deleted, err := analytics.Purge(ctx, queries, retention)
	if err != nil {
		log.Fatalf("purge: %v", err)
	}

	fmt.Printf("Deleted %d rows.\n", deleted)
}
