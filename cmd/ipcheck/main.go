package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"crow.watch/internal/dotenv"
	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dotenv.Load(".env")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ipcheck <username>\n")
		os.Exit(1)
	}
	username := os.Args[1]

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

	user, err := queries.GetUserByLogin(ctx, username)
	if err != nil {
		log.Fatalf("find user %q: %v", username, err)
	}

	// Target user info
	fmt.Printf("=== User: %s (id=%d) ===\n", user.Username, user.ID)
	if user.CreatedAt.Valid {
		fmt.Printf("Joined: %s\n", user.CreatedAt.Time.Format(time.DateOnly))
	}
	if user.Campaign != "" {
		fmt.Printf("Campaign: %s\n", user.Campaign)
	}
	if user.BannedAt.Valid {
		fmt.Printf("BANNED: %s\n", user.BannedAt.Time.Format(time.DateOnly))
	}
	if user.InviterID.Valid {
		inviter, err := queries.GetUserByID(ctx, user.InviterID.Int64)
		if err == nil {
			fmt.Printf("Invited by: %s (id=%d)\n", inviter.Username, inviter.ID)
		}
	}

	// Target user's IPs
	ips, err := queries.GetIPsByUserID(ctx, user.ID)
	if err != nil {
		log.Fatalf("get IPs: %v", err)
	}

	fmt.Printf("\n=== IPs used by %s (%d records) ===\n", user.Username, len(ips))
	curIP := ""
	for _, ip := range ips {
		if ip.IpAddress != curIP {
			curIP = ip.IpAddress
			fmt.Printf("\n  %s\n", curIP)
		}
		fmt.Printf("    %-12s  hits=%-5d  first=%s  last=%s\n",
			ip.Action, ip.HitCount,
			fmtTime(ip.FirstSeenAt.Time, ip.FirstSeenAt.Valid),
			fmtTime(ip.LastSeenAt.Time, ip.LastSeenAt.Valid),
		)
	}

	// Other users sharing IPs
	shared, err := queries.GetUsersSharingIPsWith(ctx, user.ID)
	if err != nil {
		log.Fatalf("get shared IPs: %v", err)
	}

	if len(shared) == 0 {
		fmt.Printf("\n=== No other users share IPs with %s ===\n", user.Username)
		return
	}

	// Count unique users
	seen := map[int64]bool{}
	for _, r := range shared {
		seen[r.UserID] = true
	}
	fmt.Printf("\n=== %d other user(s) sharing IPs ===\n", len(seen))

	curIP = ""
	curUser := ""
	for _, r := range shared {
		if r.IpAddress != curIP {
			curIP = r.IpAddress
			curUser = ""
			fmt.Printf("\n  %s\n", curIP)
		}
		if r.Username != curUser {
			curUser = r.Username
			extra := ""
			if r.BannedAt.Valid {
				extra += " [BANNED]"
			}
			if r.Campaign != "" {
				extra += fmt.Sprintf(" campaign=%s", r.Campaign)
			}
			invitedBy := ""
			if r.InviterName.Valid {
				invitedBy = fmt.Sprintf("  invited_by=%s", r.InviterName.String)
			}
			fmt.Printf("    %s (id=%d, joined=%s%s%s)\n",
				r.Username, r.UserID,
				fmtTime(r.UserCreatedAt.Time, r.UserCreatedAt.Valid),
				invitedBy, extra,
			)
		}
		fmt.Printf("      %-12s  hits=%-5d  first=%s  last=%s\n",
			r.Action, r.HitCount,
			fmtTime(r.FirstSeenAt.Time, r.FirstSeenAt.Valid),
			fmtTime(r.LastSeenAt.Time, r.LastSeenAt.Valid),
		)
	}
}

func fmtTime(t time.Time, valid bool) string {
	if !valid {
		return "n/a"
	}
	return t.Format(time.DateOnly)
}
