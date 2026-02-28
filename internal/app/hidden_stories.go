package app

import (
	"encoding/json"
	"net/http"
	"strconv"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) hideStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	storyID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := a.Queries.HideStory(r.Context(), store.HideStoryParams{
		UserID:  current.User.ID,
		StoryID: storyID,
	}); err != nil {
		a.serverError(w, r, "hide story", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), storyID); err != nil {
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

	storyID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := a.Queries.UnhideStory(r.Context(), store.UnhideStoryParams{
		UserID:  current.User.ID,
		StoryID: storyID,
	}); err != nil {
		a.serverError(w, r, "unhide story", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), storyID); err != nil {
		a.serverError(w, r, "recalculate story downvotes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
