package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) modUserPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := r.PathValue("username")
	target, err := a.Queries.GetUserForModeration(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get user for moderation", err)
		return
	}

	a.render(w, "mod_user", ModUserPageData{
		Base:         a.baseData(r),
		Username:     target.Username,
		IsModerator:  target.IsModerator,
		IsBanned:     target.BannedAt.Valid,
		BanReason:    target.BanReason,
		StoryCount:   target.StoryCount,
		CommentCount: target.CommentCount,
		CreatedAt:    target.CreatedAt.Time,
	})
}

func (a *App) banUser(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := r.PathValue("username")
	target, err := a.Queries.GetUserForModeration(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get user for moderation", err)
		return
	}

	if target.BannedAt.Valid {
		http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
		return
	}

	if target.IsModerator {
		http.Error(w, "cannot ban a moderator", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "(no reason given)"
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if err := qtx.BanUser(r.Context(), store.BanUserParams{
		BanReason: reason,
		ID:        target.ID,
	}); err != nil {
		a.serverError(w, r, "ban user", err)
		return
	}

	if err := qtx.DeleteSessionsByUserID(r.Context(), target.ID); err != nil {
		a.serverError(w, r, "delete sessions", err)
		return
	}

	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      "user.ban",
		TargetType:  "user",
		TargetID:    target.ID,
		Reason:      reason,
		Metadata:    []byte(fmt.Sprintf(`{"username":%q}`, target.Username)),
	}); err != nil {
		a.serverError(w, r, "create moderation log", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
}

func (a *App) unbanUser(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := r.PathValue("username")
	target, err := a.Queries.GetUserForModeration(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get user for moderation", err)
		return
	}

	if !target.BannedAt.Valid {
		http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if err := qtx.UnbanUser(r.Context(), target.ID); err != nil {
		a.serverError(w, r, "unban user", err)
		return
	}

	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      "user.unban",
		TargetType:  "user",
		TargetID:    target.ID,
		Metadata:    []byte(fmt.Sprintf(`{"username":%q}`, target.Username)),
	}); err != nil {
		a.serverError(w, r, "create moderation log", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
}

func (a *App) deleteUserStories(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := r.PathValue("username")
	target, err := a.Queries.GetUserForModeration(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get user for moderation", err)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	count, err := qtx.SoftDeleteStoriesByUser(r.Context(), target.ID)
	if err != nil {
		a.serverError(w, r, "soft delete user stories", err)
		return
	}

	if count > 0 {
		if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
			ModeratorID: current.User.ID,
			Action:      "user.delete_stories",
			TargetType:  "user",
			TargetID:    target.ID,
			Reason:      fmt.Sprintf("removed %d stories", count),
			Metadata:    []byte(fmt.Sprintf(`{"username":%q,"count":%d}`, target.Username, count)),
		}); err != nil {
			a.serverError(w, r, "create moderation log", err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
}

func (a *App) deleteUserComments(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := r.PathValue("username")
	target, err := a.Queries.GetUserForModeration(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get user for moderation", err)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	count, err := qtx.SoftDeleteCommentsByUser(r.Context(), target.ID)
	if err != nil {
		a.serverError(w, r, "soft delete user comments", err)
		return
	}

	if count > 0 {
		if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
			ModeratorID: current.User.ID,
			Action:      "user.delete_comments",
			TargetType:  "user",
			TargetID:    target.ID,
			Reason:      fmt.Sprintf("removed %d comments", count),
			Metadata:    []byte(fmt.Sprintf(`{"username":%q,"count":%d}`, target.Username, count)),
		}); err != nil {
			a.serverError(w, r, "create moderation log", err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	http.Redirect(w, r, "/mod/user/"+target.Username, http.StatusSeeOther)
}
