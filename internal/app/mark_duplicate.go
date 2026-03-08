package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) markDuplicate(w http.ResponseWriter, r *http.Request) {
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

	if row.DeletedAt.Valid {
		http.Redirect(w, r, storyPath(row.ShortCode, row.Title), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	canonicalCode := strings.TrimSpace(r.FormValue("canonical_code"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	if canonicalCode == "" {
		a.renderEditError(w, r, current, code, row, row.Title, row.Body.String, "", row.Url.String, nil, nil, "Original story short code is required.")
		return
	}

	if len(canonicalCode) != 6 {
		a.renderEditError(w, r, current, code, row, row.Title, row.Body.String, "", row.Url.String, nil, nil, "Invalid story short code.")
		return
	}

	if canonicalCode == code {
		a.renderEditError(w, r, current, code, row, row.Title, row.Body.String, "", row.Url.String, nil, nil, "A story cannot be a duplicate of itself.")
		return
	}

	if reason == "" {
		reason = "(no reason given)"
	}

	// Look up the canonical story
	canonical, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: canonicalCode, Valid: true}})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.renderEditError(w, r, current, code, row, row.Title, row.Body.String, "", row.Url.String, nil, nil, fmt.Sprintf("Story with code %q not found.", canonicalCode))
			return
		}
		a.serverError(w, r, "get canonical story", err)
		return
	}

	metadata := map[string]any{
		"duplicate_of_id":         canonical.ID,
		"duplicate_of_short_code": canonical.ShortCode,
		"duplicate_of_title":      canonical.Title,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		a.serverError(w, r, "marshal metadata", err)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if err := qtx.MarkStoryDuplicate(r.Context(), store.MarkStoryDuplicateParams{
		DuplicateOfID: pgtype.Int8{Int64: canonical.ID, Valid: true},
		ID:            row.ID,
	}); err != nil {
		a.serverError(w, r, "mark story duplicate", err)
		return
	}

	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      "story.mark_duplicate",
		TargetType:  "story",
		TargetID:    row.ID,
		Reason:      reason,
		Metadata:    metadataJSON,
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

func (a *App) unmarkDuplicate(w http.ResponseWriter, r *http.Request) {
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

	if !row.DuplicateOfID.Valid {
		http.Redirect(w, r, storyPath(row.ShortCode, row.Title), http.StatusSeeOther)
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

	if err := qtx.UnmarkStoryDuplicate(r.Context(), row.ID); err != nil {
		a.serverError(w, r, "unmark story duplicate", err)
		return
	}

	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      "story.unmark_duplicate",
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
