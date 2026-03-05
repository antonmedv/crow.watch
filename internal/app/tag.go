package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/store"
)

// tagPage serves the hotness-ranked story listing for a specific tag
// (GET /t/{tag} and GET /t/{tag}/page/{page}).
func (a *App) tagPage(w http.ResponseWriter, r *http.Request) {
	tagName := r.PathValue("tag")
	if tagName == "" {
		http.NotFound(w, r)
		return
	}

	tag, err := a.Queries.GetTagByName(r.Context(), tagName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get tag by name", err)
		return
	}

	page := parsePage(r)
	data := TagPageData{
		BaseData:       a.baseData(r),
		TagName:        tag.Tag,
		TagDescription: tag.Description,
		CurrentPage:    page,
		PagePath:       fmt.Sprintf("/t/%s/page", tag.Tag),
	}

	stories, hasMore, err := a.loadStoryList(r, data.BaseData, page, store.ListStoriesParams{
		TagID:      pgtype.Int8{Int64: tag.ID, Valid: true},
		StoryLimit: 500,
	}, storyListOpts{rankByHotness: true, filterNegScore: true, filterHidden: true})
	if err != nil {
		a.serverError(w, r, "load stories", err)
		return
	}

	data.Stories = stories
	data.HasMore = hasMore
	a.render(w, "tag", data)
}
