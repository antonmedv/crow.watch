package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

type commentVoteResponse struct {
	OK    bool `json:"ok"`
	Score int  `json:"score"`
}

func (a *App) upvoteComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	comment, err := a.Queries.GetCommentByID(r.Context(), commentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if comment.UserID == current.User.ID {
		http.Error(w, "cannot vote on own comment", http.StatusForbidden)
		return
	}

	score, err := a.Queries.CreateCommentVote(r.Context(), store.CreateCommentVoteParams{
		UserID:    current.User.ID,
		CommentID: commentID,
	})
	if err != nil {
		a.serverError(w, r, "create comment vote", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commentVoteResponse{OK: true, Score: int(score)})
}

func (a *App) unvoteComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	score, err := a.Queries.DeleteCommentVote(r.Context(), store.DeleteCommentVoteParams{
		UserID:    current.User.ID,
		CommentID: commentID,
	})
	if err != nil {
		a.serverError(w, r, "delete comment vote", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commentVoteResponse{OK: true, Score: int(score)})
}

func (a *App) flagComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
	for _, fr := range flagReasons {
		if fr == req.Reason {
			validReason = true
			break
		}
	}
	if !validReason {
		http.Error(w, "invalid flag reason", http.StatusBadRequest)
		return
	}

	score, err := a.Queries.CreateCommentFlag(r.Context(), store.CreateCommentFlagParams{
		UserID:    current.User.ID,
		CommentID: commentID,
		Reason:    req.Reason,
	})
	if err != nil {
		a.serverError(w, r, "create comment flag", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commentVoteResponse{OK: true, Score: int(score)})
}

func (a *App) unflagComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	score, err := a.Queries.DeleteCommentFlag(r.Context(), store.DeleteCommentFlagParams{
		UserID:    current.User.ID,
		CommentID: commentID,
	})
	if err != nil {
		a.serverError(w, r, "delete comment flag", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commentVoteResponse{OK: true, Score: int(score)})
}
