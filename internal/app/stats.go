package app

import (
	"net/http"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

type ModStatsPageData struct {
	Base  Base
	Stats store.GetSiteStatsRow
}

func (a *App) modStatsPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	stats, err := a.Queries.GetSiteStats(r.Context())
	if err != nil {
		a.serverError(w, r, "get site stats", err)
		return
	}

	a.render(w, "mod_stats", ModStatsPageData{
		Base:  a.baseData(r),
		Stats: stats,
	})
}
