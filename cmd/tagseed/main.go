package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// mediaCategories contains category names whose tags get is_media=true.
var mediaCategories = map[string]bool{
	"format": true,
}

// privilegedCategories contains category names whose tags get privileged=true.
var privilegedCategories = map[string]bool{
	"crow": true,
}

func main() {
	loadDotEnv(".env")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: tagseed <tags.yaml>\n")
		os.Exit(1)
	}

	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}

	// Parse: map of category name → map of tag name → description
	var spec map[string]map[string]string
	if err := yaml.Unmarshal(data, &spec); err != nil {
		log.Fatalf("parse yaml: %v", err)
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

	// Sort categories for deterministic output
	catNames := make([]string, 0, len(spec))
	for name := range spec {
		catNames = append(catNames, name)
	}
	sort.Strings(catNames)

	var totalTags int
	for _, catName := range catNames {
		tags := spec[catName]

		// Get or create category
		cat, err := getOrCreateCategory(ctx, queries, catName)
		if err != nil {
			log.Fatalf("category %q: %v", catName, err)
		}

		isMedia := mediaCategories[catName]
		privileged := privilegedCategories[catName]

		// Sort tags for deterministic output
		tagNames := make([]string, 0, len(tags))
		for name := range tags {
			tagNames = append(tagNames, name)
		}
		sort.Strings(tagNames)

		for _, tagName := range tagNames {
			desc := tags[tagName]
			err := queries.UpsertTag(ctx, store.UpsertTagParams{
				Tag:         tagName,
				Description: desc,
				CategoryID:  pgtype.Int8{Int64: cat.ID, Valid: true},
				Privileged:  privileged,
				IsMedia:     isMedia,
			})
			if err != nil {
				log.Fatalf("tag %q: %v", tagName, err)
			}
			totalTags++
		}

		fmt.Printf("  %s: %d tags\n", catName, len(tags))
	}

	fmt.Printf("Seeded %d tags across %d categories.\n", totalTags, len(catNames))
}

func getOrCreateCategory(ctx context.Context, q *store.Queries, name string) (store.Category, error) {
	cat, err := q.GetCategoryByName(ctx, name)
	if err == nil {
		return cat, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Category{}, err
	}
	return q.CreateCategory(ctx, name)
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
