package app

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) hideStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: r.PathValue("code"), Valid: true}})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := a.Queries.HideStory(r.Context(), store.HideStoryParams{
		UserID:  current.User.ID,
		StoryID: story.ID,
	}); err != nil {
		a.serverError(w, r, "hide story", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), story.ID); err != nil {
		a.serverError(w, r, "recalculate story downvotes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (a *App) unhideStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: r.PathValue("code"), Valid: true}})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := a.Queries.UnhideStory(r.Context(), store.UnhideStoryParams{
		UserID:  current.User.ID,
		StoryID: story.ID,
	}); err != nil {
		a.serverError(w, r, "unhide story", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), story.ID); err != nil {
		a.serverError(w, r, "recalculate story downvotes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
