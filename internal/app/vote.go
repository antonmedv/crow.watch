package app

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

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

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: r.PathValue("code"), Valid: true}})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	upvotes, err := a.Queries.CreateVote(r.Context(), store.CreateVoteParams{
		UserID:  current.User.ID,
		StoryID: story.ID,
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

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: r.PathValue("code"), Valid: true}})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	upvotes, err := a.Queries.DeleteVote(r.Context(), store.DeleteVoteParams{
		UserID:  current.User.ID,
		StoryID: story.ID,
	})
	if err != nil {
		a.serverError(w, r, "delete vote", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voteResponse{OK: true, Upvotes: int(upvotes)})
}
