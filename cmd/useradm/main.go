package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	loadDotEnv(".env")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: useradm <add|passwd> [flags]\n")
		os.Exit(1)
	}

	subcmd := os.Args[1]

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

	switch subcmd {
	case "add":
		cmdAdd(ctx, queries, os.Args[2:])
	case "passwd":
		cmdPasswd(ctx, queries, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", subcmd)
		os.Exit(1)
	}
}

func cmdAdd(ctx context.Context, q *store.Queries, args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	username := fs.String("username", "", "username for the new user")
	email := fs.String("email", "", "email for the new user")
	inviterID := fs.Int64("inviter-id", 0, "user ID of the inviter (optional)")
	fs.Parse(args)

	if *username == "" || *email == "" {
		fmt.Fprintf(os.Stderr, "usage: useradm add -username <name> -email <email> [-inviter-id <id>]\n")
		os.Exit(1)
	}

	password, err := readPasswordConfirm("Password: ", "Confirm password: ")
	if err != nil {
		log.Fatalf("read password: %v", err)
	}

	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	params := store.CreateUserParams{
		Username:       *username,
		Email:          *email,
		PasswordDigest: string(digest),
	}
	if *inviterID != 0 {
		params.InviterID = pgtype.Int8{Int64: *inviterID, Valid: true}
	}

	user, err := q.CreateUser(ctx, params)
	if err != nil {
		log.Fatalf("create user: %v", err)
	}

	fmt.Printf("Created user: id=%d username=%s email=%s\n", user.ID, user.Username, user.Email)
}

func cmdPasswd(ctx context.Context, q *store.Queries, args []string) {
	fs := flag.NewFlagSet("passwd", flag.ExitOnError)
	login := fs.String("user", "", "username or email of the user")
	fs.Parse(args)

	if *login == "" {
		fmt.Fprintf(os.Stderr, "usage: useradm passwd -user <username|email>\n")
		os.Exit(1)
	}

	user, err := q.GetUserByLogin(ctx, *login)
	if err != nil {
		log.Fatalf("find user: %v", err)
	}

	password, err := readPasswordConfirm("New password: ", "Confirm new password: ")
	if err != nil {
		log.Fatalf("read password: %v", err)
	}

	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	err = q.UpdateUserPasswordByID(ctx, store.UpdateUserPasswordByIDParams{
		PasswordDigest: string(digest),
		ID:             user.ID,
	})
	if err != nil {
		log.Fatalf("update password: %v", err)
	}

	fmt.Printf("Password updated for %s (id=%d)\n", user.Username, user.ID)
}

func readPasswordConfirm(prompt1, prompt2 string) (string, error) {
	p1, err := readPassword(prompt1)
	if err != nil {
		return "", err
	}
	p2, err := readPassword(prompt2)
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", fmt.Errorf("passwords do not match")
	}
	if p1 == "" {
		return "", fmt.Errorf("password must not be empty")
	}
	if len(p1) > 72 {
		return "", fmt.Errorf("password must be 72 bytes or fewer")
	}
	return p1, nil
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(pw), nil
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
