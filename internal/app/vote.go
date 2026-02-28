package app

import (
	"encoding/json"
	"net/http"
	"strconv"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

type voteResponse struct {
	OK      bool `json:"ok"`
	Upvotes int  `json:"upvotes"`
}

func (a *App) upvote(w http.ResponseWriter, r *http.Request) {
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

	upvotes, err := a.Queries.CreateVote(r.Context(), store.CreateVoteParams{
		UserID:  current.User.ID,
		StoryID: storyID,
	})
	if err != nil {
		a.serverError(w, r, "create vote", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voteResponse{OK: true, Upvotes: int(upvotes)})
}

func (a *App) unvote(w http.ResponseWriter, r *http.Request) {
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

	upvotes, err := a.Queries.DeleteVote(r.Context(), store.DeleteVoteParams{
		UserID:  current.User.ID,
		StoryID: storyID,
	})
	if err != nil {
		a.serverError(w, r, "delete vote", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voteResponse{OK: true, Upvotes: int(upvotes)})
}
