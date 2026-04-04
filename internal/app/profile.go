package app

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"crow.watch/internal/auth"
)

func (a *App) profilePage(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		http.NotFound(w, r)
		return
	}

	profile, err := a.Queries.GetPublicProfile(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Moderators can still view banned user profiles
			current, ok := auth.UserFromContext(r.Context())
			if ok && current.User.IsModerator {
				http.Redirect(w, r, "/mod/user/"+username, http.StatusSeeOther)
				return
			}
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get public profile", err)
		return
	}

	var invitedBy string
	if profile.InviterName.Valid {
		invitedBy = profile.InviterName.String
	}

	a.render(w, "profile", ProfilePageData{
		Base:            a.baseData(r),
		ProfileUsername: profile.Username,
		About:           profile.About,
		Website:         profile.Website,
		IsModerator:     profile.IsModerator,
		StoryCount:      profile.StoryCount,
		InvitedBy:       invitedBy,
		CreatedAt:       profile.CreatedAt.Time,
	})
}
