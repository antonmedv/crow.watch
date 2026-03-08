package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/store"
)

func (a *App) userStoriesPage(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		http.NotFound(w, r)
		return
	}

	_, err := a.Queries.GetPublicProfile(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get public profile", err)
		return
	}

	page := parsePage(r)
	data := UserStoriesPageData{
		Base:            a.baseData(r),
		ProfileUsername: username,
		CurrentPage:     page,
		PagePath:        fmt.Sprintf("/u/%s/stories/page", username),
	}

	stories, hasMore, err := a.loadStoryList(r, data.Base, page, store.ListStoriesParams{
		Username:   pgtype.Text{String: username, Valid: true},
		StoryLimit: 500,
	}, storyListOpts{})
	if err != nil {
		a.serverError(w, r, "load stories", err)
		return
	}

	data.Stories = stories
	data.HasMore = hasMore
	a.render(w, "user-stories", data)
}
