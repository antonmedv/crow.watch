package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const backupPath = "/data/backup.sql.gz"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	tokenHash := os.Getenv("TOKEN_HASH")
	if tokenHash == "" {
		logger.Error("TOKEN_HASH is required")
		os.Exit(1)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	var mu sync.Mutex

	runBackup := func() {
		mu.Lock()
		defer mu.Unlock()

		logger.Info("starting pg_dump")
		start := time.Now()

		cmd := exec.Command("sh", "-c", fmt.Sprintf("pg_dump '%s' | gzip > '%s'", databaseURL, backupPath))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			logger.Error("pg_dump failed", "err", err)
			return
		}

		info, err := os.Stat(backupPath)
		if err != nil {
			logger.Error("stat failed", "err", err)
			return
		}
		logger.Info("backup complete", "duration", time.Since(start).Round(time.Second).String(), "bytes", info.Size())
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
			logger.Warn("unauthorized request", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		h := sha256.Sum256([]byte(token))
		if hex.EncodeToString(h[:]) != tokenHash {
			logger.Warn("invalid token", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		logger.Info("serving backup", "remote", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/gzip")
		http.ServeFile(w, r, backupPath)
	})

	logger.Info("listening", "addr", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
