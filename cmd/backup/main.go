package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const backupPath = "/data/backup.sql.gz"

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	tokenHash := os.Getenv("TOKEN_HASH")
	if tokenHash == "" {
		log.Fatal("TOKEN_HASH is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	var mu sync.Mutex

	runBackup := func() {
		mu.Lock()
		defer mu.Unlock()

		log.Println("backup: starting pg_dump...")
		start := time.Now()

		cmd := exec.Command("sh", "-c", fmt.Sprintf("pg_dump '%s' | gzip > '%s'", databaseURL, backupPath))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("backup: pg_dump failed: %v", err)
			return
		}

		info, err := os.Stat(backupPath)
		if err != nil {
			log.Printf("backup: stat failed: %v", err)
			return
		}
		log.Printf("backup: done in %s (%d bytes)", time.Since(start).Round(time.Second), info.Size())
	}

	// Run immediately on startup.
	runBackup()

	// Schedule daily at 00:00 UTC.
	go func() {
		for {
			now := time.Now().UTC()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			time.Sleep(next.Sub(now))
			runBackup()
		}
	}()

	http.HandleFunc("GET /backup.sql.gz", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		h := sha256.Sum256([]byte(token))
		if hex.EncodeToString(h[:]) != tokenHash {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/gzip")
		http.ServeFile(w, r, backupPath)
	})

	log.Printf("backup: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
