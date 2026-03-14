package app

import (
	"net/http"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) hideTag(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tag, err := a.Queries.GetTagByName(r.Context(), r.PathValue("tag"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := a.Queries.HideTag(r.Context(), store.HideTagParams{
		UserID: current.User.ID,
		TagID:  tag.ID,
	}); err != nil {
		a.serverError(w, r, "hide tag", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (a *App) unhideTag(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tag, err := a.Queries.GetTagByName(r.Context(), r.PathValue("tag"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := a.Queries.UnhideTag(r.Context(), store.UnhideTagParams{
		UserID: current.User.ID,
		TagID:  tag.ID,
	}); err != nil {
		a.serverError(w, r, "unhide tag", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
