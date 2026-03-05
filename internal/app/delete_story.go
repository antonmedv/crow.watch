package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) deleteStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	code := r.PathValue("code")
	if len(code) != 6 {
		http.NotFound(w, r)
		return
	}

	row, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: code, Valid: true}})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get story by short code", err)
		return
	}

	// Already deleted — just redirect back
	if row.DeletedAt.Valid {
		http.Redirect(w, r, storyPath(row.ShortCode, row.Title), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "(no reason given)"
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if err := qtx.SoftDeleteStory(r.Context(), row.ID); err != nil {
		a.serverError(w, r, "soft delete story", err)
		return
	}

	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      "story.delete",
		TargetType:  "story",
		TargetID:    row.ID,
		Reason:      reason,
		Metadata:    []byte("{}"),
	}); err != nil {
		a.serverError(w, r, "create moderation log", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	http.Redirect(w, r, storyPath(row.ShortCode, row.Title), http.StatusSeeOther)
}
