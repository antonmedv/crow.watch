package main

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	"crow.watch/internal/link"
	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

type seedStory struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title"`
}

func main() {
	loadDotEnv(".env")

	storiesPath := "stories.yaml"
	count := 25

	if len(os.Args) >= 2 {
		n, err := strconv.Atoi(os.Args[1])
		if err == nil {
			count = n
		} else {
			storiesPath = os.Args[1]
		}
	}
	if len(os.Args) >= 3 {
		n, err := strconv.Atoi(os.Args[2])
		if err == nil {
			count = n
		}
	}

	data, err := os.ReadFile(storiesPath)
	if err != nil {
		log.Fatalf("read stories %s: %v", storiesPath, err)
	}

	var stories []seedStory
	if err := yaml.Unmarshal(data, &stories); err != nil {
		log.Fatalf("parse yaml: %v", err)
	}

	if count > len(stories) {
		count = len(stories)
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

	// Get or create a seed user.
	user, err := getOrCreateSeedUser(ctx, queries)
	if err != nil {
		log.Fatalf("seed user: %v", err)
	}
	fmt.Printf("Using user %q (id=%d)\n", user.Username, user.ID)

	// Load existing tags for random assignment.
	tags, err := queries.ListActiveTags(ctx)
	if err != nil {
		log.Fatalf("list tags: %v", err)
	}
	if len(tags) == 0 {
		fmt.Println("Warning: no active tags found. Run tagseed first to assign tags to stories.")
	}

	// Shuffle and pick stories.
	perm := rand.Perm(len(stories))
	selected := make([]seedStory, count)
	for i := range count {
		selected[i] = stories[perm[i]]
	}

	var created int
	for i, s := range selected {
		result, err := link.Clean(s.URL)
		if err != nil {
			fmt.Printf("  skip (bad url): %s\n", s.URL)
			continue
		}

		domain, err := getOrCreateDomain(ctx, queries, result.Domain)
		if err != nil {
			log.Fatalf("domain %q: %v", result.Domain, err)
		}

		var originID pgtype.Int8
		if result.Origin != "" {
			origin, err := getOrCreateOrigin(ctx, queries, domain.ID, result.Origin)
			if err != nil {
				log.Fatalf("origin %q: %v", result.Origin, err)
			}
			originID = pgtype.Int8{Int64: origin.ID, Valid: true}
		}

		story, err := queries.CreateStory(ctx, store.CreateStoryParams{
			UserID:        user.ID,
			DomainID:      pgtype.Int8{Int64: domain.ID, Valid: true},
			OriginID:      originID,
			Url:           pgtype.Text{String: result.Cleaned, Valid: true},
			NormalizedUrl: pgtype.Text{String: result.Normalized, Valid: true},
			Title:         s.Title,
			ShortCode:     generateShortCode(),
		})
		if err != nil {
			fmt.Printf("  skip (create): %s: %v\n", s.Title, err)
			continue
		}

		_ = queries.IncrementDomainStoryCount(ctx, domain.ID)
		if originID.Valid {
			_ = queries.IncrementOriginStoryCount(ctx, originID.Int64)
		}

		// Backdate: spread stories over the last 72 hours.
		age := time.Duration(i) * (72 * time.Hour) / time.Duration(count)
		// Add jitter: ±30 minutes.
		jitter := time.Duration(rand.IntN(60)-30) * time.Minute
		backdateTo := time.Now().Add(-age + jitter)
		_, _ = pool.Exec(ctx,
			"UPDATE stories SET created_at = $1, updated_at = $1 WHERE id = $2",
			backdateTo, story.ID,
		)

		// Assign 1-3 random tags.
		if len(tags) > 0 {
			tagCount := 1 + rand.IntN(min(3, len(tags)))
			tagPerm := rand.Perm(len(tags))
			for t := range tagCount {
				_ = queries.CreateTagging(ctx, store.CreateTaggingParams{
					StoryID: story.ID,
					TagID:   tags[tagPerm[t]].ID,
				})
			}
		}

		// Auto-upvote from the author and set a random score for testing.
		_, _ = pool.Exec(ctx,
			"INSERT INTO votes (user_id, story_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			user.ID, story.ID,
		)
		seedScore := 1 + rand.IntN(30)
		_, _ = pool.Exec(ctx,
			"UPDATE stories SET score = $1 WHERE id = $2",
			seedScore, story.ID,
		)

		created++
		fmt.Printf("  [%d/%d] %s (score=%d)\n", created, count, s.Title, seedScore)
	}

	fmt.Printf("Seeded %d stories.\n", created)
}

func getOrCreateSeedUser(ctx context.Context, q *store.Queries) (store.User, error) {
	u, err := q.GetUserByLogin(ctx, "seedbot")
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, err
	}

	row, err := q.CreateUser(ctx, store.CreateUserParams{
		Username:       "seedbot",
		Email:          "seedbot@localhost",
		PasswordDigest: "!", // unusable password
	})
	if err != nil {
		return store.User{}, err
	}
	return store.User{
		ID:       row.ID,
		Username: row.Username,
		Email:    row.Email,
	}, nil
}

func getOrCreateDomain(ctx context.Context, q *store.Queries, name string) (store.Domain, error) {
	d, err := q.GetDomainByName(ctx, name)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Domain{}, err
	}
	return q.CreateDomain(ctx, name)
}

func getOrCreateOrigin(ctx context.Context, q *store.Queries, domainID int64, name string) (store.Origin, error) {
	o, err := q.GetOriginByName(ctx, name)
	if err == nil {
		return o, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Origin{}, err
	}
	return q.CreateOrigin(ctx, store.CreateOriginParams{
		DomainID: domainID,
		Origin:   name,
	})
}

func generateShortCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	if _, err := crand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

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
