package app

import (
	"html/template"
	"net/http"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/markdown"
)

type RepliesPageData struct {
	BaseData
	Replies []ReplyItem
}

type ReplyItem struct {
	CommentID     int64
	StoryTitle    string
	StoryPath     string
	CommentAuthor string
	Body          template.HTML
	CreatedAt     time.Time
	IsUnread      bool
}

func (a *App) repliesPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	rows, err := a.Queries.ListReplies(r.Context(), current.User.ID)
	if err != nil {
		a.serverError(w, r, "list replies", err)
		return
	}

	var replies []ReplyItem
	for _, r := range rows {
		replies = append(replies, ReplyItem{
			CommentID:     r.CommentID,
			StoryTitle:    r.StoryTitle,
			StoryPath:     storyPath(r.StoryShortCode, r.StoryTitle),
			CommentAuthor: r.CommentAuthor,
			Body:          markdown.Render(r.Body),
			CreatedAt:     r.CreatedAt.Time,
			IsUnread:      r.IsUnread,
		})
	}

	a.render(w, "replies", RepliesPageData{
		BaseData: a.baseData(r),
		Replies:  replies,
	})
}
