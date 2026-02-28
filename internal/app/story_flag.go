package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

var storyFlagReasons = []string{"off-topic", "already posted", "broken link", "spam"}

func (a *App) flagStory(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	validReason := false
	for _, fr := range storyFlagReasons {
		if fr == req.Reason {
			validReason = true
			break
		}
	}
	if !validReason {
		http.Error(w, "invalid flag reason", http.StatusBadRequest)
		return
	}

	if err := a.Queries.CreateStoryFlag(r.Context(), store.CreateStoryFlagParams{
		UserID:  current.User.ID,
		StoryID: storyID,
		Reason:  req.Reason,
	}); err != nil {
		a.serverError(w, r, "create story flag", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), storyID); err != nil {
		a.serverError(w, r, "recalculate story downvotes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (a *App) unflagStory(w http.ResponseWriter, r *http.Request) {
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

	if err := a.Queries.DeleteStoryFlag(r.Context(), store.DeleteStoryFlagParams{
		UserID:  current.User.ID,
		StoryID: storyID,
	}); err != nil {
		a.serverError(w, r, "delete story flag", err)
		return
	}

	if err := a.Queries.RecalculateStoryDownvotes(r.Context(), storyID); err != nil {
		a.serverError(w, r, "recalculate story downvotes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
