package app

import (
	"net/http"

	"crow.watch/internal/auth"
)

func (a *App) tagsPage(w http.ResponseWriter, r *http.Request) {
	tags, err := a.Queries.ListActiveTagsWithCategory(r.Context())
	if err != nil {
		a.serverError(w, r, "list active tags", err)
		return
	}

	base := a.baseData(r)
	isModerator := false
	var hiddenTagIDs []int64
	if current, ok := auth.UserFromContext(r.Context()); ok {
		isModerator = base.IsModerator
		ids, err := a.Queries.ListUserHiddenTagIDs(r.Context(), current.User.ID)
		if err != nil {
			a.serverError(w, r, "list hidden tags", err)
			return
		}
		hiddenTagIDs = ids
	}

	a.render(w, "tags", TagsPageData{
		BaseData:     base,
		TagGroups:    toTagGroups(tags, isModerator),
		HiddenTagIDs: hiddenTagIDs,
	})
}
