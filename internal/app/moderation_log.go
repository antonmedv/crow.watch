package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"crow.watch/internal/store"
)

const modLogPerPage = 50

func (a *App) moderationLogPage(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)

	offset := int32((page - 1) * modLogPerPage)
	rows, err := a.Queries.ListModerationLog(r.Context(), store.ListModerationLogParams{
		LogLimit:  modLogPerPage + 1,
		LogOffset: offset,
	})
	if err != nil {
		a.serverError(w, r, "list moderation log", err)
		return
	}

	hasMore := len(rows) > modLogPerPage
	if hasMore {
		rows = rows[:modLogPerPage]
	}

	var entries []ModerationLogEntry
	for _, row := range rows {
		targetLink, targetTitle := a.resolveModLogTarget(r, row.TargetType, row.TargetID)
		entries = append(entries, ModerationLogEntry{
			ID:                row.ID,
			ModeratorUsername: row.ModeratorUsername,
			Action:            row.Action,
			ActionDescription: formatActionDescription(row.Action),
			TargetType:        row.TargetType,
			TargetID:          row.TargetID,
			TargetLink:        targetLink,
			TargetTitle:       targetTitle,
			Reason:            row.Reason,
			CreatedAt:         row.CreatedAt.Time,
		})
	}

	a.render(w, "moderation_log", ModerationLogPageData{
		BaseData:    a.baseData(r),
		Entries:     entries,
		CurrentPage: page,
		HasMore:     hasMore,
	})
}

func (a *App) resolveModLogTarget(r *http.Request, targetType string, targetID int64) (link, title string) {
	if targetType == "story" {
		row, err := a.Queries.GetStoryByID(r.Context(), targetID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "[deleted]"
			}
			return "", "[error]"
		}
		return storyPath(row.ShortCode, row.Title), row.Title
	}
	return "", ""
}

func formatActionDescription(action string) string {
	parts := strings.Split(action, ",")
	var descriptions []string
	for _, p := range parts {
		switch strings.TrimSpace(p) {
		case "story.edit_url":
			descriptions = append(descriptions, "edited URL")
		case "story.edit_title":
			descriptions = append(descriptions, "edited title")
		case "story.edit_body":
			descriptions = append(descriptions, "edited body")
		case "story.edit_tags":
			descriptions = append(descriptions, "edited tags")
		default:
			descriptions = append(descriptions, strings.TrimSpace(p))
		}
	}
	return strings.Join(descriptions, ", ")
}
