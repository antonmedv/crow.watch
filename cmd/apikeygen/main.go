package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	loadDotEnv(".env")

	fs := flag.NewFlagSet("apikeygen", flag.ExitOnError)
	username := fs.String("user", "", "username of the user to generate a key for")
	name := fs.String("name", "", "label for the API key (optional)")
	fs.Parse(os.Args[1:])

	if *username == "" {
		fmt.Fprintf(os.Stderr, "usage: apikeygen -user <username> [-name <label>]\n")
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

	user, err := queries.GetUserByLogin(ctx, *username)
	if err != nil {
		log.Fatalf("find user %q: %v", *username, err)
	}

	rawToken, err := newRawToken()
	if err != nil {
		log.Fatalf("generate token: %v", err)
	}

	tokenHash := auth.HashToken(rawToken)

	key, err := queries.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		Name:      *name,
	})
	if err != nil {
		log.Fatalf("create api key: %v", err)
	}

	fmt.Printf("Created API key id=%d for user %s (name=%q)\n", key.ID, user.Username, key.Name)
	fmt.Printf("Token (save this, it will not be shown again):\n%s\n", rawToken)
}

func newRawToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
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
