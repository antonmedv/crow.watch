package app

import (
	"net/http"
	"strconv"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

const storiesPerPage = 25

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		a.notFound(w, r)
		return
	}
	a.page(w, r)
}

// page serves the hotness-ranked story listing (GET / and GET /page/{page}).
func (a *App) page(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	data := HomePageData{
		Base:        a.baseData(r),
		CurrentPage: page,
		PagePath:    "/page",
	}

	var hiddenTagIDs []int64
	if current, ok := auth.UserFromContext(r.Context()); ok {
		var err error
		hiddenTagIDs, err = a.Queries.ListUserHiddenTagIDs(r.Context(), current.User.ID)
		if err != nil {
			a.serverError(w, r, "get hidden tags", err)
			return
		}
	}

	stories, hasMore, err := a.loadStoryList(r, data.Base, page, store.ListStoriesParams{
		HideDeleted:  true,
		HiddenTagIds: hiddenTagIDs,
		StoryLimit:   500,
	}, storyListOpts{rankByHotness: true, filterNegScore: true, filterHidden: true, filterDuplicates: true})
	if err != nil {
		a.serverError(w, r, "load stories", err)
		return
	}

	data.Stories = stories
	data.HasMore = hasMore
	a.render(w, "home", data)
}

// newest serves the chronological story listing (GET /newest and GET /newest/page/{page}).
func (a *App) newest(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	data := HomePageData{
		Base:        a.baseData(r),
		CurrentPage: page,
		PagePath:    "/newest/page",
	}

	var hiddenTagIDs []int64
	if current, ok := auth.UserFromContext(r.Context()); ok {
		var err error
		hiddenTagIDs, err = a.Queries.ListUserHiddenTagIDs(r.Context(), current.User.ID)
		if err != nil {
			a.serverError(w, r, "get hidden tags", err)
			return
		}
	}

	stories, hasMore, err := a.loadStoryList(r, data.Base, page, store.ListStoriesParams{
		HideDeleted:  true,
		HiddenTagIds: hiddenTagIDs,
		StoryLimit:   500,
	}, storyListOpts{filterHidden: true, filterDuplicates: true})
	if err != nil {
		a.serverError(w, r, "load stories", err)
		return
	}

	data.Stories = stories
	data.HasMore = hasMore
	a.render(w, "home", data)
}

func parsePage(r *http.Request) int {
	pageStr := r.PathValue("page")
	if pageStr == "" {
		return 1
	}
	p, err := strconv.Atoi(pageStr)
	if err != nil || p < 1 {
		return 1
	}
	return p
}
