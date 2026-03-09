package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"crow.watch/internal/app"
	"crow.watch/internal/auth"
	"crow.watch/internal/captcha"
	"crow.watch/internal/dev"
	"crow.watch/internal/dotenv"
	"crow.watch/internal/email"
	"crow.watch/internal/ratelimit"
	"crow.watch/internal/store"
	"crow.watch/web"
)

func main() {
	dotenv.Load(".env")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping db", "error", err)
		os.Exit(1)
	}

	devMode := os.Getenv("DEV_MODE") == "1"

	var templateFS fs.FS
	var staticFS fs.FS
	if devMode {
		templateFS = os.DirFS("web")
		staticFS = os.DirFS("web/static")
	} else {
		templateFS = web.FS
		var err error
		staticFS, err = fs.Sub(web.FS, "static")
		if err != nil {
			logger.Error("static fs", "error", err)
			os.Exit(1)
		}
	}

	if dir := os.Getenv("STATIC_DIR"); dir != "" && !devMode {
		if err := exportStatic(staticFS, dir); err != nil {
			logger.Error("export static files", "error", err)
			os.Exit(1)
		}
		logger.Info("exported static files", "dir", dir)
	}

	var staticHashes map[string]string
	if !devMode {
		staticHashes, err = app.HashStatic(staticFS)
		if err != nil {
			logger.Error("hash static files", "error", err)
			os.Exit(1)
		}
	}

	templates, err := app.ParseTemplates(templateFS, staticHashes, devMode)
	if err != nil {
		logger.Error("parse templates", "error", err)
		os.Exit(1)
	}

	queries := store.New(pool)
	cookieName := envOrDefault("SESSION_COOKIE_NAME", "crowwatch_session")
	ttlHours, err := strconv.Atoi(envOrDefault("SESSION_TTL_HOURS", "720"))
	if err != nil || ttlHours <= 0 {
		logger.Error("SESSION_TTL_HOURS must be a positive integer")
		os.Exit(1)
	}

	secureCookies := envOrDefault("SECURE_COOKIES", "true") != "false" && !devMode
	sessions := auth.NewSessionManager(queries, cookieName, time.Duration(ttlHours)*time.Hour, secureCookies, logger)

	emailTemplates, err := app.ParseEmailTemplates(web.FS)
	if err != nil {
		logger.Error("parse email templates", "error", err)
		os.Exit(1)
	}

	emailSender := email.NewSender(
		envOrDefault("ZOHO_HOST", "api.zeptomail.eu"),
		os.Getenv("ZOHO_TOKEN"),
		envOrDefault("FROM_EMAIL", "noreply@crow.watch"),
		logger,
	)

	appURL := strings.TrimRight(envOrDefault("APP_URL", "http://localhost:8080"), "/")

	var devReloader *dev.Reloader
	if devMode {
		var err error
		devReloader, err = dev.NewReloader([]string{"web", "internal", "cmd"}, logger)
		if err != nil {
			logger.Error("dev reloader", "error", err)
			os.Exit(1)
		}
		defer devReloader.Close()
		go devReloader.Run()
		logger.Info("dev mode enabled")
	}

	loginIPLimiter := ratelimit.New(10, 15*time.Minute)
	loginAcctLimiter := ratelimit.New(5, 15*time.Minute)
	inviteLimiter := ratelimit.New(20, time.Hour)
	captchaStore := captcha.New(5 * time.Minute)
	rateLimitDone := make(chan struct{})
	loginIPLimiter.StartCleanup(5*time.Minute, rateLimitDone)
	loginAcctLimiter.StartCleanup(5*time.Minute, rateLimitDone)
	inviteLimiter.StartCleanup(5*time.Minute, rateLimitDone)
	captchaStore.StartCleanup(5*time.Minute, rateLimitDone)

	a := &app.App{
		Pool:             pool,
		Queries:          queries,
		Sessions:         sessions,
		Templates:        templates,
		EmailTemplates:   emailTemplates,
		EmailSender:      emailSender,
		AppURL:           appURL,
		StaticFS:         staticFS,
		Log:              logger,
		DevMode:          devMode,
		TemplateFS:       templateFS,
		DevReload:        devReloader,
		LoginIPLimiter:   loginIPLimiter,
		LoginAcctLimiter: loginAcctLimiter,
		InviteLimiter:    inviteLimiter,
		Captcha:          captchaStore,
		Visitors:         app.NewVisitorCounter(),
	}

	addr := envOrDefault("ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           a.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := queries.DeleteExpiredSessions(context.Background()); err != nil {
					logger.Error("delete expired sessions", "error", err)
				}
			case <-rateLimitDone:
				return
			}
		}
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-shutdownCh
		logger.Info("shutdown signal received", "signal", sig.String())

		close(rateLimitDone)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown", "error", err)
		}
	}()

	logger.Info("server starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func exportStatic(staticFS fs.FS, dir string) error {
	return fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(staticFS, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
